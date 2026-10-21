package controller

import (
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/model"
)

// modelsDevSnapshotBytes is replaced by scripts/generate-modelsdev.mjs during
// packaging. The catalog remains available without runtime network access.
//go:embed modelsdev_snapshot.json
var modelsDevSnapshotBytes []byte

type modelsDevSnapshot struct {
	SourceURL   string           `json:"source_url"`
	GeneratedAt string           `json:"generated_at"`
	Models      []upstreamModel  `json:"models"`
	Vendors     []upstreamVendor `json:"vendors"`
}

func bundledModelsDevCatalog(locale string, raw []byte) (metadataSyncSource, map[string]model.MetadataValues, map[string]model.Vendor, error) {
	source := metadataSyncSource{
		Source:     "modelsdev",
		Locale:     locale,
		ModelsURL:  "https://models.dev/api.json (bundled at build)",
		VendorsURL: "https://models.dev/api.json (bundled at build)",
		Version:    fmt.Sprintf("%x", sha256.Sum256(raw)),
	}
	var snapshot modelsDevSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return source, nil, nil, fmt.Errorf("invalid bundled models.dev metadata: %w", err)
	}
	if len(snapshot.Models) == 0 {
		return source, nil, nil, errors.New("models.dev snapshot is empty; run the build-time metadata generator")
	}
	if strings.TrimSpace(snapshot.SourceURL) != "https://models.dev/api.json" {
		return source, nil, nil, errors.New("unexpected models.dev snapshot source")
	}
	source.GeneratedAt = snapshot.GeneratedAt
	models := make(map[string]model.MetadataValues, len(snapshot.Models))
	vendors := make(map[string]model.Vendor, len(snapshot.Vendors))
	for _, item := range snapshot.Vendors {
		if name := strings.TrimSpace(item.Name); name != "" {
			if _, exists := vendors[name]; exists {
				return source, nil, nil, fmt.Errorf("duplicate bundled vendor %q", name)
			}
			vendors[name] = model.Vendor{Name: name, Description: item.Description, Icon: item.Icon, Status: 1}
		}
	}
	for _, item := range snapshot.Models {
		name := strings.TrimSpace(item.ModelName)
		if name == "" || len(name) > 128 {
			return source, nil, nil, errors.New("invalid models.dev model name")
		}
		if _, exists := models[name]; exists {
			return source, nil, nil, fmt.Errorf("duplicate bundled model %q", name)
		}
		vendor := strings.TrimSpace(item.VendorName)
		if vendor != "" {
			if _, exists := vendors[vendor]; !exists {
				return source, nil, nil, fmt.Errorf("missing bundled vendor %q for model %q", vendor, name)
			}
		}
		values := model.MetadataValues{
			Description: item.Description, Icon: item.Icon, Tags: item.Tags,
			Vendor: vendor, Endpoints: "", NameRule: 0, Status: item.Status,
		}
		if err := model.ValidateMetadataValues(values); err != nil {
			return source, nil, nil, fmt.Errorf("invalid bundled metadata for %q: %w", name, err)
		}
		models[name] = values
	}
	return source, models, vendors, nil
}
