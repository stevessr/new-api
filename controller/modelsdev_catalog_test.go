package controller

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testModelsDevSnapshot() []byte {
	return []byte(`{
		"source_url": "https://models.dev/api.json",
		"generated_at": "2026-10-21T00:00:00Z",
		"vendors": [{"name":"OpenAI","status":1}],
		"models": [{
			"model_name":"gpt-test",
			"description":"Sample model",
			"vendor_name":"OpenAI",
			"tags":"models.dev,reasoning",
			"status":1
		}]
	}`)
}

func TestBundledModelsDevCatalog(t *testing.T) {
	source, models, vendors, err := bundledModelsDevCatalog("zh", testModelsDevSnapshot())
	require.NoError(t, err)
	assert.Equal(t, "modelsdev", source.Source)
	assert.Equal(t, "zh", source.Locale)
	assert.Equal(t, "2026-10-21T00:00:00Z", source.GeneratedAt)
	assert.NotEmpty(t, source.Version)
	assert.Contains(t, source.ModelsURL, "(bundled at build)")
	assert.Equal(t, "OpenAI", models["gpt-test"].Vendor)
	assert.Equal(t, "", models["gpt-test"].Endpoints)
	assert.Equal(t, 1, models["gpt-test"].Status)
	assert.Contains(t, vendors, "OpenAI")

	second := testModelsDevSnapshot()
	second[len(second)-2] = ' '
	next, _, _, err := bundledModelsDevCatalog("zh", second)
	require.NoError(t, err)
	assert.NotEqual(t, source.Version, next.Version)
}

func TestBundledModelsDevCatalogRejectsInvalidData(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw []byte
	}{
		{"empty", []byte(`{"source_url":"https://models.dev/api.json","models":[]}`)},
		{"invalid json", []byte("garbage")},
		{"unknown source", []byte(strings.ReplaceAll(string(testModelsDevSnapshot()), "https://models.dev/api.json", "https://unknown.example"))},
		{"missing vendor", []byte(strings.ReplaceAll(string(testModelsDevSnapshot()), "\"OpenAI\"", "\"missing\""))},
		{"duplicate", func() []byte {
			var snapshot modelsDevSnapshot
			if err := json.Unmarshal(testModelsDevSnapshot(), &snapshot); err != nil { panic(err) }
			snapshot.Models = append(snapshot.Models, snapshot.Models[0])
			data, _ := json.Marshal(snapshot)
			return data
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := bundledModelsDevCatalog("en", tc.raw)
			require.Error(t, err)
		})
	}
}
