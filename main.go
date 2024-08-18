package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
)

func main() {
	ImageResult = make(map[string]ImageData)
	InitPython()
	defer FinalizePython()

	e := echo.New()
	e.POST("/upload", Upload)
	e.GET("/check/:id", Check)
	e.GET("/result/:id", Result)
	e.GET("/image/:id", ServeImage)
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal("error starting service: " + err.Error())
	}
	err = http.ListenAndServe(fmt.Sprintf(":%s", os.Getenv("PORT")), e)
	if err != nil {
		log.Fatal("error starting service: " + err.Error())
	}
}
