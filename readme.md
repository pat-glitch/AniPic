# AniPic

## Overview

AniPic is a web application that allows users to search their Google Photos for animal pictures and generate animated images from these pictures. The application is built using Golang and leverages multiple Google Cloud services for authentication, storage, and deployment.

## Features

- **Google OAuth 2.0 Authentication**: Securely authenticate users with their Google account to access Google Photos.
- **Upload straight from device
- **Animation Generation**: Generate animations from the retrieved animal photos.
- **Cloud Hosting**: Host the frontend on Google Cloud Storage and deploy the backend using Google Cloud Run.

## Services Used

- **Google Drive API**: To be able to store the generated images/gifs to the user's google drive.
- **Google Cloud Storage**: To host the frontend and store animation files.
- **Google Cloud Functions**: For generating animations from the retrieved photos.
- **Google Cloud Run**: To deploy and run the backend Golang server.
- **Google OAuth 2.0**: For secure user authentication.
