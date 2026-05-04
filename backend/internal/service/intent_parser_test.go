package service

import (
	"context"
	"testing"
	"time"
)

func TestParseSearchIntentExtractsDateAndExtension(t *testing.T) {
	now := time.Date(2026, 5, 4, 15, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	intent := ParseSearchIntentWithOptions(context.Background(), "找一下三个月前上传的 pdf", nil, SearchIntentOptions{
		Now:      now,
		Timezone: "Asia/Shanghai",
	})

	if intent.TextQuery != "" {
		t.Fatalf("expected empty text query, got %q", intent.TextQuery)
	}
	if len(intent.Extensions) != 1 || intent.Extensions[0] != "pdf" {
		t.Fatalf("expected pdf extension, got %#v", intent.Extensions)
	}
	if len(intent.MimeTypes) != 1 || intent.MimeTypes[0] != "application/pdf" {
		t.Fatalf("expected pdf mime, got %#v", intent.MimeTypes)
	}
	wantFrom := time.Date(2026, 2, 4, 0, 0, 0, 0, now.Location()).UTC()
	if intent.DateFrom == nil || !intent.DateFrom.Equal(wantFrom) {
		t.Fatalf("expected date_from %s, got %v", wantFrom, intent.DateFrom)
	}
	if intent.DateTo == nil || !intent.DateTo.Equal(now.UTC()) {
		t.Fatalf("expected date_to %s, got %v", now.UTC(), intent.DateTo)
	}
}

func TestParseSearchIntentExtractsTodayXLSX(t *testing.T) {
	now := time.Date(2026, 5, 4, 9, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	intent := ParseSearchIntentWithOptions(context.Background(), "今天上传的 xlsx 文件", nil, SearchIntentOptions{
		Now:      now,
		Timezone: "Asia/Shanghai",
	})

	if intent.TextQuery != "" {
		t.Fatalf("expected empty text query, got %q", intent.TextQuery)
	}
	if len(intent.Extensions) != 1 || intent.Extensions[0] != "xlsx" {
		t.Fatalf("expected xlsx extension, got %#v", intent.Extensions)
	}
	wantFrom := time.Date(2026, 5, 4, 0, 0, 0, 0, now.Location()).UTC()
	wantTo := time.Date(2026, 5, 4, 23, 59, 59, int(time.Second-time.Nanosecond), now.Location()).UTC()
	if intent.DateFrom == nil || !intent.DateFrom.Equal(wantFrom) {
		t.Fatalf("expected today start, got %v", intent.DateFrom)
	}
	if intent.DateTo == nil || !intent.DateTo.Equal(wantTo) {
		t.Fatalf("expected today end, got %v", intent.DateTo)
	}
}

func TestParseSearchIntentKeepsPureSemanticQuery(t *testing.T) {
	intent := ParseSearchIntentWithOptions(context.Background(), "关于机器学习的论文", nil, SearchIntentOptions{
		Now:      time.Date(2026, 5, 4, 9, 0, 0, 0, time.UTC),
		Timezone: "Asia/Shanghai",
	})

	if intent.TextQuery != "关于机器学习的论文" {
		t.Fatalf("expected original semantic query, got %q", intent.TextQuery)
	}
	if intent.HasFilters() {
		t.Fatalf("expected no filters, got %#v", intent)
	}
}

func TestParseSearchIntentDoesNotTreatFilenameAsExtensionFilter(t *testing.T) {
	intent := ParseSearchIntentWithOptions(context.Background(), "contract.pdf", nil, SearchIntentOptions{
		Now:      time.Date(2026, 5, 4, 9, 0, 0, 0, time.UTC),
		Timezone: "Asia/Shanghai",
	})

	if intent.TextQuery != "contract.pdf" {
		t.Fatalf("expected filename query to stay intact, got %q", intent.TextQuery)
	}
	if intent.HasFilters() {
		t.Fatalf("expected no filters for filename, got %#v", intent)
	}
}

func TestParseSearchIntentExtractsFuzzyMediaType(t *testing.T) {
	now := time.Date(2026, 5, 4, 9, 0, 0, 0, time.UTC)
	intent := ParseSearchIntentWithOptions(context.Background(), "最近的照片", nil, SearchIntentOptions{
		Now:      now,
		Timezone: "UTC",
	})

	if intent.TextQuery != "" {
		t.Fatalf("expected empty text query, got %q", intent.TextQuery)
	}
	if len(intent.MimeTypes) != 1 || intent.MimeTypes[0] != "image/" {
		t.Fatalf("expected image mime prefix, got %#v", intent.MimeTypes)
	}
	if intent.DateFrom == nil || !intent.DateFrom.Equal(now.AddDate(0, 0, -7)) {
		t.Fatalf("expected recent week date_from, got %v", intent.DateFrom)
	}
}
