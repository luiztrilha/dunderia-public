package team

import (
	"strings"
	"testing"
)

func TestNormalizeTaskArtifactBrowserInspectionPackagesEvidence(t *testing.T) {
	artifact, err := normalizeTaskArtifact(taskArtifact{
		Kind:  "browser inspection",
		Title: "Checkout CTA inspection",
		BrowserInspection: &taskBrowserInspection{
			PageURL:        " http://localhost:7891/#/channels/general ",
			Selector:       " [data-testid=\"checkout-cta\"] ",
			ElementText:    "Finalizar compra",
			ScreenshotPath: " D:\\tmp\\checkout-cta.png ",
			ViewportWidth:  -390,
			ViewportHeight: 844,
		},
	}, "human", "2026-05-05T12:00:00Z", func() string { return "artifact-browser" })
	if err != nil {
		t.Fatalf("normalize browser inspection artifact: %v", err)
	}
	if artifact.Kind != "browser_inspection" || artifact.ResultRole != "evidence" {
		t.Fatalf("unexpected browser artifact role/kind: %+v", artifact)
	}
	if artifact.URL != "http://localhost:7891/#/channels/general" || artifact.PreviewURL != artifact.URL {
		t.Fatalf("expected browser page URL to populate URL fields: %+v", artifact)
	}
	if artifact.Path != "D:\\tmp\\checkout-cta.png" {
		t.Fatalf("expected screenshot path as artifact path, got %+v", artifact)
	}
	if artifact.BrowserInspection == nil || artifact.BrowserInspection.Selector != "[data-testid=\"checkout-cta\"]" {
		t.Fatalf("expected normalized browser inspection payload: %+v", artifact)
	}
	if artifact.BrowserInspection.ViewportWidth != 0 || artifact.BrowserInspection.ViewportHeight != 844 {
		t.Fatalf("expected viewport normalization, got %+v", artifact.BrowserInspection)
	}
	if !strings.Contains(artifact.Summary, "selector=[data-testid=\"checkout-cta\"]") || !strings.Contains(artifact.Summary, "text=Finalizar compra") {
		t.Fatalf("expected synthesized browser summary, got %q", artifact.Summary)
	}
}
