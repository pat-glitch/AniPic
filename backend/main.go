package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var (
	googleOauthConfig = &oauth2.Config{
		RedirectURL:  "http://localhost:8080/callback",
		ClientID:     "",
		ClientSecret: "",
		Scopes:       []string{"https://www.googleapis.com/auth/photoslibrary.readonly"},
		Endpoint:     google.Endpoint,
	}
	oauthStateString = "random"
)

func main() {
	r := gin.Default()
	r.GET("/", handleHome)
	r.GET("/login", handleGoogleLogin)
	r.GET("/callback", handleGoogleCallback)
	r.Run(":8080")
}

func handleHome(c *gin.Context) {
	html := `<html><body><a href="/login">Google Log In</a></body></html>`
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

func handleGoogleLogin(c *gin.Context) {
	url := googleOauthConfig.AuthCodeURL(oauthStateString)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func handleGoogleCallback(c *gin.Context) {
	state := c.Query("state")
	if state != oauthStateString {
		log.Println("invalid oauth state")
		c.Redirect(http.StatusTemporaryRedirect, "/")
		return
	}

	code := c.Query("code")
	token, err := googleOauthConfig.Exchange(oauth2.NoContext, code)
	if err != nil {
		log.Println("code exchange failed")
		c.Redirect(http.StatusTemporaryRedirect, "/")
		return
	}

	client := googleOauthConfig.Client(oauth2.NoContext, token)
	photos, err := getAnimalPhotos(client)
	if err != nil {
		log.Println("failed to get photos")
		c.Redirect(http.StatusTemporaryRedirect, "/")
		return
	}

	// Process photos(Store in Cloud Storage, create animations, etc.)
	c.JSON(http.StatusOK, photos)
}

func getAnimalPhotos(client *http.Client) ([]string, error) {
	// Implement the API call to Google Photos Library to search for animal photos
	// Parse the response and return the photo URLs or IDs

	req, err := http.NewRequest("GET", "https://photoslibrary.googleapis.com/v1/mediaItems:search", nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Add("query", "animal")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Authorization", "Bearer "+client.Transport.(*oauth2.Transport).Source.Token().AccessToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		MediaItems []struct {
			BaseURL string `json:"baseUrl"`
		} `json:"mediaItems"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var photos []string
	for _, item := range result.MediaItems {
		photos = append(photos, item.BaseURL)
	}
	return photos, nil
}
