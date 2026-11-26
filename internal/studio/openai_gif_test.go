package studio

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/gif"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
)

func animatedGIF(t *testing.T) []byte {
	t.Helper()
	palette := color.Palette{color.Black, color.White}
	first := image.NewPaletted(image.Rect(0, 0, 16, 16), palette)
	second := image.NewPaletted(first.Bounds(), palette)
	second.SetColorIndex(8, 8, 1)
	var encoded bytes.Buffer
	err := gif.EncodeAll(&encoded, &gif.GIF{
		Image: []*image.Paletted{first, second}, Delay: []int{20, 20}, LoopCount: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func TestOpenAIGIFGenerationAndReferencesPreserveAnimation(t *testing.T) {
	for _, references := range []int{0, 2} {
		t.Run(strconv.Itoa(references)+" references", func(t *testing.T) {
			testOpenAIGIF(t, references)
		})
	}
}

func testOpenAIGIF(t *testing.T, references int) {
	original, animation := originalPNG(t), animatedGIF(t)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if references == 0 {
			var fields map[string]any
			if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
				t.Error(err)
			}
			if r.URL.Path != "/v1/images/generations" || fields["output_format"] != "gif" || fields["size"] != "512x512" || fields["frame_count"] != float64(2) || fields["frame_duration_ms"] != float64(200) {
				t.Errorf("GIF generation parameters must retain numeric types: %+v", fields)
			}
		} else {
			fields, files := readOpenAIRequest(t, r)
			if r.URL.Path != "/v1/images/edits" || fields["output_format"] != "gif" || fields["size"] != "512x512" || fields["frame_count"] != "2" || fields["frame_duration_ms"] != "200" || len(files["image[]"]) != references {
				t.Errorf("GIF edit parameters or references were lost: %+v", fields)
			}
			for _, file := range files["image[]"] {
				if !bytes.Equal(file, original) {
					t.Error("GIF reference was re-encoded")
				}
			}
		}
		// Omitting response output_format exercises the requested-format fallback.
		json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(animation)}}})
	}))
	defer provider.Close()
	app, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	configureOpenAI(t, app, provider.URL+"/v1")
	session, input := openAIInput(t, app, references, original, original)
	input.Prompt, input.MarkupID = "让灯光闪烁，平滑循环", ""
	input.Options = Options{Count: 2, Format: "gif", Size: "512x512", FrameCount: 2, FrameDurationMS: 200}
	response := localRequest(t, app, "POST", "/api/sessions/"+session.ID+"/turns", input)
	if response.Code != http.StatusAccepted {
		t.Fatal(response.Body.String())
	}
	app.wg.Wait()
	saved, err := app.store.session(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	turn := saved.Turns[0]
	if turn.Status != "completed" || turn.Completed != 2 || len(turn.Outputs) != 2 || turn.Options.FrameCount != 2 || turn.Options.FrameDurationMS != 200 {
		t.Fatalf("GIF result or persisted options lost: %+v", turn)
	}
	for _, output := range turn.Outputs {
		asset, _ := assetByID(saved, output.AssetID)
		if asset.MIME != "image/gif" || filepath.Ext(asset.Path) != ".gif" || filepath.Ext(asset.Name) != ".gif" || asset.Width != 16 || asset.Height != 16 {
			t.Fatalf("GIF asset metadata lost: %+v", asset)
		}
		for _, suffix := range []string{"", "?download=1"} {
			response := localRequest(t, app, "GET", "/api/sessions/"+session.ID+"/assets/"+asset.ID+suffix, nil)
			if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "image/gif" || !bytes.Equal(response.Body.Bytes(), animation) {
				t.Fatal("GIF preview/download must preserve every frame, delay and loop byte")
			}
		}
	}
}

func TestOpenAIGIFOptions(t *testing.T) {
	defaults := GenerationInput{Prompt: "动画", Options: Options{Format: "gif"}}
	if err := validateOpenAIInput(&defaults); err != nil || defaults.Options.FrameCount != 9 || defaults.Options.FrameDurationMS != 200 || defaults.Options.Size != "auto" {
		t.Fatalf("incorrect GIF defaults: %+v, %v", defaults.Options, err)
	}
	for _, options := range []Options{
		{Format: "gif", Size: "512x512", FrameCount: 2, FrameDurationMS: 20},
		{Format: "gif", Size: "768x512", FrameCount: 16, FrameDurationMS: 21},
		{Format: "gif", Size: "auto", FrameCount: 9, FrameDurationMS: 2001},
	} {
		input := GenerationInput{Prompt: "动画", Options: options}
		if err := validateOpenAIInput(&input); err != nil {
			t.Errorf("documented GIF options rejected: %+v, %v", options, err)
		}
	}
	for _, options := range []Options{
		{Format: "gif", FrameCount: 1}, {Format: "gif", FrameCount: 17},
		{Format: "gif", FrameDurationMS: 19}, {Format: "gif", Size: "513x512"},
		{Format: "gif", Size: "0x512"}, {Format: "gif", Size: "512"},
		{Format: "gif", Background: "transparent"},
	} {
		input := GenerationInput{Prompt: "动画", Options: options}
		if validateOpenAIInput(&input) == nil {
			t.Errorf("unsupported GIF options accepted: %+v", options)
		}
	}
	still := GenerationInput{Prompt: "静态图片", Options: Options{Format: "png", Size: "512x512"}}
	if validateOpenAIInput(&still) == nil {
		t.Fatal("GIF support changed static-image size validation")
	}
}
