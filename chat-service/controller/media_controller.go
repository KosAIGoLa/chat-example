package controller

import (
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"

	"ws-ex/dto"
	"ws-ex/service"
)

// MediaController handles voice/media upload and download.
type MediaController struct {
	media *service.MediaService
}

// NewMediaController creates a MediaController.
func NewMediaController(media *service.MediaService) *MediaController {
	return &MediaController{media: media}
}

// UploadVoice accepts a multipart voice file.
// POST /api/voice  form fields: file (required), duration (optional seconds)
func (ctrl *MediaController) UploadVoice(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{
			Code:    400,
			Message: "file is required (multipart field name: file)",
		})
		return
	}
	if fileHeader.Size > service.MaxVoiceBytes {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{
			Code:    400,
			Message: "file too large (max 5MB)",
		})
		return
	}

	var duration float64
	if d := c.PostForm("duration"); d != "" {
		if v, err := strconv.ParseFloat(d, 64); err == nil {
			duration = v
		}
	}

	src, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.APIResponseDTO{
			Code:    500,
			Message: "failed to open upload",
		})
		return
	}
	defer src.Close()

	contentType := fileHeader.Header.Get("Content-Type")
	// Browsers often send "video/webm" for audio-only MediaRecorder output,
	// or omit type entirely / use application/octet-stream.
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = service.ContentTypeForFilename(fileHeader.Filename)
	}
	// Normalize video/webm → treat as voice webm (common Chrome quirk).
	if contentType == "video/webm" || contentType == "video/webm;codecs=opus" {
		contentType = "audio/webm"
	}

	// Multipart Size may be 0/-1 for streamed blobs; SaveVoice counts actual bytes.
	declaredSize := fileHeader.Size
	if declaredSize < 0 {
		declaredSize = 0
	}

	id, url, mimeType, size, err := ctrl.media.SaveVoice(src, declaredSize, contentType, duration)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.APIResponseDTO{
			Code:    400,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, dto.APIResponseDTO{
		Code:    200,
		Message: "uploaded",
		Data: dto.VoiceUploadResponse{
			ID:       id,
			URL:      url,
			MimeType: mimeType,
			Size:     size,
			Duration: duration,
		},
	})
}

// GetVoice streams a stored voice file.
// GET /api/voice/:filename
func (ctrl *MediaController) GetVoice(c *gin.Context) {
	filename := filepath.Base(c.Param("filename"))
	path, err := ctrl.media.ResolvePath(filename)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.APIResponseDTO{
			Code:    404,
			Message: "voice not found",
		})
		return
	}

	c.Header("Content-Type", service.ContentTypeForFilename(filename))
	c.Header("Cache-Control", "private, max-age=86400")
	c.Header("Accept-Ranges", "bytes")
	c.File(path)
}
