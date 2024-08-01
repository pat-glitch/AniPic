package main

import (
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"io"
	"net/http"
	"os"

	"cloud.google.com/go/storage"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

var (
	oauthConfig  *oauth2.Config
	bucketName   string
	driveService *drive.Service
)

func init() {
	creds, err := os.ReadFile("credentials.json")
	if err != nil {
		fmt.Printf("Unable to read client secret file: %v", err)
		return
	}

	oauthConfig, err = google.ConfigFromJSON(creds, drive.DriveFileScope)
	if err != nil {
		fmt.Printf("Unable to parse client secret file to config: %v", err)
		return
	}

	bucketName = os.Getenv("BUCKET_NAME")
	if bucketName == "" {
		bucketName = "YOUR_CLOUD_STORAGE_BUCKET_NAME"
	}
}

func main() {
	router := gin.Default()

	router.Static("/static", "./static")

	router.GET("/login", handleGoogleLogin)
	router.GET("/oauth2callback", handleGoogleCallback)
	router.POST("/upload", handleUpload)
	router.POST("/animate", handleAnimate)
	router.GET("/download/:filename", handleDownload)

	router.Run(":8080")
}

func handleGoogleLogin(c *gin.Context) {
	url := oauthConfig.AuthCodeURL("state", oauth2.AccessTypeOffline)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func handleGoogleCallback(c *gin.Context) {
	code := c.Query("code")
	token, err := oauthConfig.Exchange(context.Background(), code)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to exchange token: %v", err)
		return
	}

	client := oauthConfig.Client(context.Background(), token)
	driveService, err = drive.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		c.String(http.StatusInternalServerError, "Unable to retrieve Drive client: %v", err)
		return
	}

	c.Redirect(http.StatusTemporaryRedirect, "/static/index.html")
}

func handleUpload(c *gin.Context) {
	form, err := c.MultipartForm()
	if err != nil {
		c.String(http.StatusBadRequest, "Failed to parse form: %v", err)
		return
	}

	files := form.File["images"]
	var imageUrls []string
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to create storage client: %v", err)
		return
	}
	defer client.Close()

	for _, file := range files {
		src, err := file.Open()
		if err != nil {
			c.String(http.StatusBadRequest, "Failed to open file: %v", err)
			return
		}
		defer src.Close()

		filename := uuid.New().String() + ".jpg"
		wc := client.Bucket(bucketName).Object(filename).NewWriter(ctx)
		if _, err = io.Copy(wc, src); err != nil {
			c.String(http.StatusInternalServerError, "Failed to write to storage: %v", err)
			return
		}
		if err := wc.Close(); err != nil {
			c.String(http.StatusInternalServerError, "Failed to close storage writer: %v", err)
			return
		}
		imageUrls = append(imageUrls, fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucketName, filename))
	}

	c.JSON(http.StatusOK, gin.H{"imageUrls": imageUrls})
}

func handleAnimate(c *gin.Context) {
	var req struct {
		ImageUrls []string `json:"imageUrls"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.String(http.StatusBadRequest, "Failed to bind request: %v", err)
		return
	}

	if len(req.ImageUrls) == 0 {
		c.String(http.StatusBadRequest, "No images provided")
		return
	}

	var images []image.Image
	for _, url := range req.ImageUrls {
		resp, err := http.Get(url)
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to fetch image: %v", err)
			return
		}
		defer resp.Body.Close()

		img, _, err := image.Decode(resp.Body)
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to decode image: %v", err)
			return
		}
		images = append(images, img)
	}

	outGif := &gif.GIF{}
	for _, img := range images {
		bounds := img.Bounds()
		palettedImage := image.NewPaletted(bounds, nil)
		draw.Draw(palettedImage, palettedImage.Rect, img, bounds.Min, draw.Over)
		outGif.Image = append(outGif.Image, palettedImage)
		outGif.Delay = append(outGif.Delay, 0)
	}

	filename := uuid.New().String() + ".gif"
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to create storage client: %v", err)
		return
	}
	defer client.Close()

	wc := client.Bucket(bucketName).Object(filename).NewWriter(ctx)
	if err := gif.EncodeAll(wc, outGif); err != nil {
		c.String(http.StatusInternalServerError, "Failed to encode GIF: %v", err)
		return
	}
	if err := wc.Close(); err != nil {
		c.String(http.StatusInternalServerError, "Failed to close storage writer: %v", err)
		return
	}

	if driveService != nil {
		f := &drive.File{
			Name:    filename,
			Parents: []string{"root"},
		}
		file, err := os.Open(filename)
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to open file: %v", err)
			return
		}
		defer file.Close()

		_, err = driveService.Files.Create(f).Media(file).Do()
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to upload to Drive: %v", err)
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"animationUrl": fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucketName, filename),
		"downloadUrl":  fmt.Sprintf("/download/%s", filename),
	})
}

func handleDownload(c *gin.Context) {
	filename := c.Param("filename")
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to create storage client: %v", err)
		return
	}
	defer client.Close()

	rc, err := client.Bucket(bucketName).Object(filename).NewReader(ctx)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to read from storage: %v", err)
		return
	}
	defer rc.Close()

	c.Writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	if _, err = io.Copy(c.Writer, rc); err != nil {
		c.String(http.StatusInternalServerError, "Failed to copy to response: %v", err)
		return
	}
}
