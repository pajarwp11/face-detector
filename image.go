package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/go-python3"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type GeneralResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type UploadResponse struct {
	GeneralResponse
	ID string `json:"id"`
}

type CheckResponse struct {
	GeneralResponse
	FaceCount int `json:"face_count"`
}

type ResultResponse struct {
	GeneralResponse
	ImageURL string `json:"image_url"`
}

type ImageData struct {
	FileName      string
	FileExtension string
	FaceCount     int
	IsProcessed   bool
}

var imageType = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
}

var ImageResult map[string]ImageData
var mu sync.Mutex
var detectFacesFunc *python3.PyObject

func Upload(c echo.Context) error {
	file, err := c.FormFile("image")
	var res UploadResponse
	if err != nil {
		res.Code = http.StatusBadRequest
		res.Message = "error read file: " + err.Error()
		return c.JSON(http.StatusBadRequest, res)
	}

	openedFile, err := file.Open()
	if err != nil {
		res.Code = http.StatusBadRequest
		res.Message = "error open file: " + err.Error()
		return c.JSON(http.StatusBadRequest, res)
	}
	defer openedFile.Close()

	buffer := make([]byte, 512)
	_, err = openedFile.Read(buffer)
	if err != nil {
		res.Code = http.StatusInternalServerError
		res.Message = "error read file: " + err.Error()
		return c.JSON(http.StatusInternalServerError, res)
	}

	fileType := http.DetectContentType(buffer)
	_, ok := imageType[fileType]
	if !ok {
		res.Code = http.StatusUnsupportedMediaType
		res.Message = "uploaded file must be an image"
		return c.JSON(http.StatusBadRequest, res)
	}

	_, err = openedFile.Seek(0, io.SeekStart)
	if err != nil {
		res.Code = http.StatusInternalServerError
		res.Message = "error reset file pointer: " + err.Error()
		return c.JSON(http.StatusInternalServerError, res)
	}

	id := uuid.New().String()
	filePath := filepath.Join("uploads", id+filepath.Ext(file.Filename))
	dst, err := os.Create(filePath)
	if err != nil {
		res.Code = http.StatusInternalServerError
		res.Message = "error save file: " + err.Error()
		return c.JSON(http.StatusInternalServerError, res)
	}
	defer dst.Close()
	_, err = io.Copy(dst, openedFile)
	if err != nil {
		res.Code = http.StatusInternalServerError
		res.Message = "error save file: " + err.Error()
		return c.JSON(http.StatusInternalServerError, res)
	}

	newImageData := ImageData{
		FileName:      id,
		FileExtension: filepath.Ext(file.Filename),
	}
	ImageResult[id] = newImageData

	res.Code = http.StatusOK
	res.Message = "image uploaded"
	res.ID = id
	return c.JSON(http.StatusOK, res)
}

func Check(c echo.Context) error {
	id := c.Param("id")
	var res CheckResponse
	val, ok := ImageResult[id]
	if !ok {
		res.Code = http.StatusNotFound
		res.Message = "image id not exist"
		return c.JSON(http.StatusNotFound, res)
	}

	if val.IsProcessed {
		res.Code = http.StatusOK
		res.Message = "image id found"
		res.FaceCount = val.FaceCount
		return c.JSON(http.StatusOK, res)
	}

	faceCount, err := DetectFace(val.FileName, val.FileExtension)
	if err != nil {
		res.Code = http.StatusInternalServerError
		res.Message = "error face detect: " + err.Error()
	}
	ImageResult[id] = ImageData{
		FileName:      val.FileName,
		FileExtension: val.FileExtension,
		FaceCount:     faceCount,
		IsProcessed:   true,
	}
	res.Code = http.StatusOK
	res.Message = "image id found"
	res.FaceCount = faceCount
	return c.JSON(http.StatusOK, res)
}

func Result(c echo.Context) error {
	id := c.Param("id")
	var res ResultResponse
	val, ok := ImageResult[id]
	if !ok {
		res.Code = http.StatusNotFound
		res.Message = "image id not exist"
		return c.JSON(http.StatusNotFound, res)
	}

	if !val.IsProcessed {
		faceCount, err := DetectFace(val.FileName, val.FileExtension)
		if err != nil {
			res.Code = http.StatusInternalServerError
			res.Message = "error face detect: " + err.Error()
		}
		ImageResult[id] = ImageData{
			FileName:      val.FileName,
			FileExtension: val.FileExtension,
			FaceCount:     faceCount,
			IsProcessed:   true,
		}
	}
	expiry := os.Getenv("URL_EXPIRY")
	timeExpiry, err := strconv.Atoi(expiry)
	if err != nil {
		timeExpiry = 15
	}
	urlExpiration := time.Now().Add(time.Duration(timeExpiry) * time.Minute).Unix()
	signedURL := fmt.Sprintf("http://localhost:8000/image/%s?expired=%d", id, urlExpiration)
	res.Code = http.StatusOK
	res.Message = "image id found"
	res.ImageURL = signedURL
	return c.JSON(http.StatusOK, res)
}

func ServeImage(c echo.Context) error {
	id := c.Param("id")
	var res ResultResponse
	val, ok := ImageResult[id]
	if !ok {
		res.Code = http.StatusNotFound
		res.Message = "image id not exist"
		return c.JSON(http.StatusNotFound, res)
	}
	expiry := c.QueryParam("expired")
	expirationTime, err := strconv.Atoi(expiry)
	if err != nil {
		res.Code = http.StatusBadRequest
		res.Message = "error convert expiration time: " + err.Error()
		return c.JSON(http.StatusBadRequest, res)
	}
	if time.Now().Unix() > int64(expirationTime) {
		res.Code = http.StatusBadRequest
		res.Message = "url expired"
		return c.JSON(http.StatusBadRequest, res)
	}
	filePath := filepath.Join("uploads", id+val.FileExtension)
	return c.File(filePath)
}

func DetectFace(id string, extension string) (int, error) {
	mu.Lock()
	defer mu.Unlock()
	path := filepath.Join("uploads", id+extension)
	pypath := python3.PyUnicode_FromString(path)
	args := python3.PyTuple_New(1)
	python3.PyTuple_SetItem(args, 0, pypath)
	result := detectFacesFunc.Call(args, &python3.PyObject{})
	if result == nil {
		return 0, errors.New("error call detect face function")
	}
	if !python3.PyLong_Check(result) {
		return 0, errors.New("unexpected result type")
	}
	faceCount := python3.PyLong_AsLong(result)
	return faceCount, nil
}

func InitPython() {
	python3.Py_Initialize()
	scriptDir := python3.PyUnicode_FromString(".")
	sysPath := python3.PySys_GetObject("path")
	python3.PyList_Append(sysPath, scriptDir)
	faceDetectorModule := python3.PyImport_ImportModule("face_detector")
	detectFacesFunc = faceDetectorModule.GetAttrString("detect_faces")
}

func FinalizePython() {
	python3.Py_Finalize()
}
