package vision

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
	"github.com/trithemius/parq-vision/internal/config"
)

type VisionClient struct {
	client *openai.Client
	model  string
}

func NewVisionClient(cfg config.LLMConfig) *VisionClient {
	c := openai.DefaultConfig(cfg.APIKey)
	c.BaseURL = cfg.BaseURL
	return &VisionClient{
		client: openai.NewClientWithConfig(c),
		model:  cfg.Model,
	}
}

func (c *VisionClient) DescribeImage(imagePath string, prompt string, maxPixels int) (string, error) {
	imgData, err := os.ReadFile(imagePath)
	if err != nil {
		return "", fmt.Errorf("failed to read image file: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(imagePath))
	var mimeType string
	switch ext {
	case ".png":
		mimeType = "image/png"
	case ".jpg", ".jpeg":
		mimeType = "image/jpeg"
	case ".webp":
		mimeType = "image/webp"
	default:
		mimeType = "image/jpeg"
	}

	// Resize if needed (in-memory only)
	var resizedData []byte
	if maxPixels > 0 {
		resizedData, mimeType, err = resizeImageIfNeeded(imgData, mimeType, maxPixels)
		if err != nil {
			return "", fmt.Errorf("failed to resize image: %w", err)
		}
	} else {
		resizedData = imgData
	}

	base64Data := base64.StdEncoding.EncodeToString(resizedData)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64Data)

	msg := openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role: openai.ChatMessageRoleUser,
				MultiContent: []openai.ChatMessagePart{
					{
						Type: openai.ChatMessagePartTypeText,
						Text: prompt,
					},
					{
						Type: openai.ChatMessagePartTypeImageURL,
						ImageURL: &openai.ChatMessageImageURL{
							URL: dataURL,
						},
					},
				},
			},
		},
	}

	// Retry logic
	var resp openai.ChatCompletionResponse
	for i := 0; i < 3; i++ {
		resp, err = c.client.CreateChatCompletion(context.Background(), msg)
		if err == nil {
			return resp.Choices[0].Message.Content, nil
		}
		
		if i < 2 {
			time.Sleep(time.Duration(math.Pow(2, float64(i))) * time.Second)
		}
	}

	return "", fmt.Errorf("API call failed after retries: %w", err)
}

func resizeImageIfNeeded(data []byte, mimeType string, maxPixels int) ([]byte, string, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", err
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	pixels := width * height

	if pixels <= maxPixels {
		return data, mimeType, nil
	}

	scale := math.Sqrt(float64(maxPixels) / float64(pixels))
	newWidth := int(float64(width) * scale)
	newHeight := int(float64(height) * scale)

	newImg := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	// Composite on white background
	draw.Draw(newImg, newImg.Bounds(), image.White, image.Point{}, draw.Src)
	draw.BiLinear.Scale(newImg, newImg.Bounds(), img, img.Bounds(), draw.Over, nil)

	var buf bytes.Buffer
	// Encode as JPEG for space efficiency in API call
	err = jpeg.Encode(&buf, newImg, &jpeg.Options{Quality: 85})
	return buf.Bytes(), "image/jpeg", err
}
