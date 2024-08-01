package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/gif"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/gin-gonic/gin"
	"golang.org/x/image/draw"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

var (
	bucketName   = "your-bucket-name"
	oauthConfig  *oauth2.Config
	driveService *drive.Service
)

const credFilePath = "credentials.json"

func init() {
	b, err := os.ReadFile(credFilePath)
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	config, err := google.ConfigFromJSON(b, drive.DriveFileScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	oauthConfig = config
}

func main() {
	router := gin.Default()

	router.GET("/login", handleGoogleLogin)
	router.GET("/oauth2callback", handleGoogleCallback)
	router.POST("/upload", handleUpload)
	router.POST("/animate", handleAnimate)
	router.GET("/download/:filename", handleDownload)

	router.Run(":8080")
}

func handleGoogleLogin(c *gin.Context) {
	url := oauthConfig.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func handleGoogleCallback(c *gin.Context) {
	code := c.Query("code")

	token, err := oauthConfig.Exchange(context.Background(), code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to exchange token"})
		return
	}

	client := oauthConfig.Client(context.Background(), token)
	srv, err := drive.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to create Drive client"})
		return
	}
	driveService = srv

	c.JSON(http.StatusOK, gin.H{"message": "Login successful"})
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

	if driveService != nil {
		err := saveToDrive(animation, "animation.gif")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save to Google Drive"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"animationUrl": animationURL, "downloadUrl": fmt.Sprintf("/download/%s", "animation.gif")})
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

func saveToDrive(fileContent []byte, filename string) error {
	fileMetadata := &drive.File{
		Name:    filename,
		Parents: []string{"root"},
	}
	file := bytes.NewReader(fileContent)

	_, err := driveService.Files.Create(fileMetadata).Media(file).Do()
	return err
}

func handleDownload(c *gin.Context) {
	filename := c.Param("filename")
	filePath := filepath.Join(os.TempDir(), filename)

	f, err := os.Open(filePath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}
	defer f.Close()

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "application/octet-stream")
	c.File(filePath)
}
