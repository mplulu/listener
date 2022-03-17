package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"runtime"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/mplulu/log"
)

const (
	kPort     = ":28897"
	BodyField = "body"
)

type Dict map[string]interface{}
type ErrToRespFunc func(err error) Dict

func main() {
	go startServer()
	select {}
}

func startServer() {
	r := echo.New()
	r.HTTPErrorHandler = GenCustomHttpErrorHandler(func(err error) Dict {
		return Dict{
			"err": err.Error(),
		}
	})
	r.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
	}))
	r.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: "[${host}] time=${time_rfc3339}, duration=${latency_human}, status=${status}, method=${method}, uri=${uri}\n",
	}))
	r.Use(CustomRecover("PAZEMO", func(err error) Dict {
		return Dict{
			"err": err.Error(),
		}
	}))
	r.Use(PreRequest)
	r.Use(middleware.BodyDump(GenCustomBodyDumpHandler("pazemo")))
	r.POST("/*", listenerHandler)
	r.GET("/*", listenerHandler)

	// Start server
	err := r.Start(kPort)
	if err != nil {
		panic(err)
	}
}

func listenerHandler(c echo.Context) error {
	return c.JSON(200, Dict{
		"success": true,
	})
}

func PreRequest(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Read the content
		req := c.Request()

		var bodyBytes []byte
		if req.Body != nil {
			bodyBytes, _ = ioutil.ReadAll(req.Body)
			// Restore the io.ReadCloser to its original state
			req.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
			c.Set(BodyField, string(bodyBytes))
		}

		return next(c)
	}
}

func GenCustomBodyDumpHandler(provider string) middleware.BodyDumpHandler {
	return func(c echo.Context, reqBody, resBody []byte) {
		log.Log("<<<<<<< %v-x START %s: %s", provider, c.Request().Method, c.Request().RequestURI)
		reqStr := strings.Trim(string(reqBody), "\n\t ")
		resStr := strings.Trim(string(resBody), "\n\t ")
		log.Log("=> %v-x: request body: %s", provider, reqStr)
		log.Log("=> %v-x: response status: %d, body: %s", provider, c.Response().Status, resStr)
		log.Log(">>>>>>> %v-x END -----", provider)
	}
}

func CustomRecover(provider string, panicErrToRespFunc ErrToRespFunc) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			defer func() {
				if r := recover(); r != nil {
					err, ok := r.(error)
					if !ok {
						err = fmt.Errorf("%v", r)
					}
					log.Log("<<<<<<< %v-x START RECOVER %s: %s", provider, c.Request().Method, c.Request().RequestURI)
					reqBody := c.Get(BodyField).(string)
					log.Log("=> %v-x: request body: %s", provider, reqBody)
					stack := make([]byte, 4<<10)
					length := runtime.Stack(stack, false)
					msg := fmt.Sprintf("PanicRecover %v %s", err, stack[:length])
					msg = strings.Trim(msg, "\n\t ")
					log.Log(msg)
					resp := Dict{
						"err": "err:internal_error",
					}
					if panicErrToRespFunc != nil {
						resp = panicErrToRespFunc(err)
					}
					c.JSON(http.StatusInternalServerError, resp)
					log.Log("=> %v-x: response body: %v", provider, resp)
					log.Log(">>>>>>> %v-x END RECOVER", provider)
				}
			}()
			return next(c)
		}
	}
}

func GenCustomHttpErrorHandler(errToRespFunc ErrToRespFunc) echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}
		body := Dict{
			"err": err.Error(),
		}
		if errToRespFunc != nil {
			body = errToRespFunc(err)
		}
		c.JSON(http.StatusOK, body)
	}
}
