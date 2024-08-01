package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/gif"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/gin-gonic/gin"
	"golang.org/x/image/draw"
)

var bucketName = "your-bucket-name"

func main() {
	router := gin.Default()

	router.POST("/upload", handleUpload)
	router.POST("/animate", handleAnimate)

	router.Run(":8080")
}

func handleUpload(c *gin.Context) {
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	files := form.File["images"]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No images uploaded"})
		return
	}

	var imageUrls []string
	errors := make(chan error, len(files))
	urls := make(chan string, len(files))

	for _, file := range files {
		go func(file *multipart.FileHeader) {
			filePath, err := saveFile(c, file)
			if err != nil {
				errors <- err
				return
			}

			imageUrl, err := uploadToGCS(filePath)
			if err != nil {
				errors <- err
				return
			}
			urls <- imageUrl

			// Clean up the local file
			os.Remove(filePath)
		}(file)
	}

	for i := 0; i < len(files); i++ {
		select {
		case err := <-errors:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		case url := <-urls:
			imageUrls = append(imageUrls, url)
		}
	}

	c.JSON(http.StatusOK, gin.H{"imageUrls": imageUrls})
}

func saveFile(c *gin.Context, file *multipart.FileHeader) (string, error) {
	if !isValidImage(file.Filename) {
		return "", fmt.Errorf("invalid file type")
	}

	dst := filepath.Join(os.TempDir(), file.Filename)
	if err := c.SaveUploadedFile(file, dst); err != nil {
		return "", err
	}
	return dst, nil
}

func isValidImage(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif":
		return true
	}
	return false
}

func uploadToGCS(filePath string) (string, error) {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return "", err
	}
	defer client.Close()

	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	wc := client.Bucket(bucketName).Object(filepath.Base(filePath)).NewWriter(ctx)
	buf := make([]byte, 1024*1024) // 1MB buffer
	for {
		n, err := f.Read(buf)
		if err != nil && err != io.EOF {
			return "", err
		}
		if n == 0 {
			break
		}
		if _, err := wc.Write(buf[:n]); err != nil {
			return "", err
		}
	}
	if err := wc.Close(); err != nil {
		return "", err
	}

	return fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucketName, filepath.Base(filePath)), nil
}

func handleAnimate(c *gin.Context) {
	var request struct {
		ImageUrls []string `json:"imageUrls"`
	}
	if err := c.BindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	animation, err := createAnimation(request.ImageUrls)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create animation"})
		return
	}

	animationURL, err := uploadAnimationToGCS(animation)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload animation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"animationUrl": animationURL})
}

func createAnimation(imageUrls []string) ([]byte, error) {
	var images []*image.Paletted
	var delays []int

	for _, url := range imageUrls {
		resp, err := http.Get(url)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		img, _, err := image.Decode(resp.Body)
		if err != nil {
			return nil, err
		}

		palettedImg := image.NewPaletted(img.Bounds(), nil)
		draw.FloydSteinberg.Draw(palettedImg, img.Bounds(), img, image.Point{})

		images = append(images, palettedImg)
		delays = append(delays, 100) // 100ms delay between frames
	}

	outGif := &gif.GIF{
		Image: images,
		Delay: delays,
	}

	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, outGif); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func uploadAnimationToGCS(animation []byte) (string, error) {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return "", err
	}
	defer client.Close()

	objectName := "animation.gif"
	wc := client.Bucket(bucketName).Object(objectName).NewWriter(ctx)
	if _, err := wc.Write(animation); err != nil {
		return "", err
	}
	if err := wc.Close(); err != nil {
		return "", err
	}

	return fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucketName, objectName), nil
}
