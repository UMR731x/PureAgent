package studio

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func TestRequestPreservesPromptAndAutomaticChoices(t *testing.T) {
	input := GenerationInput{Prompt: "  透明玻璃小鸟\n保留手写的细节  ", Options: Options{Count: 2}}
	if err := validateInput(&input); err != nil {
		t.Fatal(err)
	}
	encoded, err := requestBody(Model{ModelName: "doubao-seedream-5.5"}, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	json.Unmarshal(encoded, &body)
	if body["prompt"] != input.Prompt {
		t.Fatal("user prompt was changed")
	}
	for _, prohibited := range []string{"messages", "system", "size", "background", "n", "stream", "sequential_image_generation"} {
		if _, present := body[prohibited]; present {
			t.Errorf("unsupported or automatic field was sent: %s", prohibited)
		}
	}
}

func TestLayerRequestUsesDocumentedFields(t *testing.T) {
	input := GenerationInput{ReferenceIDs: []string{"reference"}, Options: Options{
		LayerDecomposition: true, MaxImages: 100, TextRecognition: true, Background: "transparent",
	}}
	if err := validateInput(&input); err != nil {
		t.Fatal(err)
	}
	encoded, err := requestBody(Model{ModelName: "doubao-seedream-5.5"}, input, []string{"data:image/png;base64,original"})
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	json.Unmarshal(encoded, &body)
	if _, present := body["background"]; present {
		t.Fatal("background is forbidden in layer decomposition")
	}
	layers := body["layer_decomposition_options"].(map[string]any)
	if layers["max_images"] != float64(100) || layers["text_recognition"] != true || body["image"] != "data:image/png;base64,original" {
		t.Fatalf("unexpected layer request: %s", encoded)
	}
}

func TestSupportedRatiosStayWithinPixelBudget(t *testing.T) {
	for _, resolution := range []string{"auto", "1K", "1.5K", "2K"} {
		for _, ratio := range []string{"1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3", "21:9", "9:21", "16:1", "1:16"} {
			size, err := imageSize(Options{Resolution: resolution, Ratio: ratio})
			if err != nil {
				t.Fatal(err)
			}
			parts := strings.Split(size, "x")
			width, _ := strconv.Atoi(parts[0])
			height, _ := strconv.Atoi(parts[1])
			pixels := width * height
			if pixels < 921600 || pixels > 4624220 {
				t.Errorf("%s %s produced unsupported size %s", resolution, ratio, size)
			}
		}
	}
}
