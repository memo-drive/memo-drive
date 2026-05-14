package store

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"unicode"

	"github.com/memodrive/backend/internal/indexing"
	"github.com/memodrive/backend/internal/model"
)

const defaultChunkSnippetLength = 240

type ChunkRow struct {
	ID            string
	FileID        string
	FileName      string
	Heading       string
	ChunkIndex    int
	Text          string
	ParentChunkID string
	IsParent      bool
}

func (s *Store) UpsertIndexChunks(ctx context.Context, records []indexing.ChunkRecord) error {
	rows := make([]ChunkRow, 0, len(records))
	for _, record := range records {
		rows = append(rows, ChunkRow{
			ID:            record.ID,
			FileID:        record.FileID,
			FileName:      record.FileName,
			Heading:       record.Heading,
			ChunkIndex:    record.ChunkIndex,
			Text:          record.Text,
			ParentChunkID: record.ParentChunkID,
			IsParent:      record.IsParent,
		})
	}
	return s.UpsertChunks(ctx, rows)
}

func (s *Store) UpsertChunks(ctx context.Context, rows []ChunkRow) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	fileIDs := make(map[string]struct{})
	for _, row := range rows {
		if row.FileID != "" {
			fileIDs[row.FileID] = struct{}{}
		}
	}
	for fileID := range fileIDs {
		if _, err = tx.ExecContext(ctx, `DELETE FROM chunks WHERE file_id = ?`, fileID); err != nil {
			return err
		}
	}

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO chunks (id, file_id, file_name, heading, chunk_index, text, parent_chunk_id, is_parent, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(id) DO UPDATE SET
    file_id = excluded.file_id,
    file_name = excluded.file_name,
    heading = excluded.heading,
    chunk_index = excluded.chunk_index,
    text = excluded.text,
    parent_chunk_id = excluded.parent_chunk_id,
    is_parent = excluded.is_parent,
    updated_at = CURRENT_TIMESTAMP`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, row := range rows {
		if strings.TrimSpace(row.ID) == "" || strings.TrimSpace(row.FileID) == "" {
			continue
		}
		if _, err = stmt.ExecContext(ctx,
			row.ID,
			row.FileID,
			row.FileName,
			row.Heading,
			row.ChunkIndex,
			row.Text,
			row.ParentChunkID,
			row.IsParent,
		); err != nil {
			return err
		}
	}
	err = tx.Commit()
	return err
}

func (s *Store) DeleteChunksByFileID(ctx context.Context, fileID string) error {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM chunks WHERE file_id = ?`, fileID)
	return err
}

func (s *Store) GetChunkText(ctx context.Context, id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", ErrNotFound
	}
	var text string
	err := s.db.QueryRowContext(ctx, `SELECT text FROM chunks WHERE id = ?`, id).Scan(&text)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", ErrNotFound
		}
		return "", err
	}
	return text, nil
}

func (s *Store) SearchChunksBM25(ctx context.Context, query string, fileIDs []string, limit int) ([]model.SourceChunk, error) {
	rawQuery := strings.TrimSpace(query)
	if rawQuery == "" || limit <= 0 {
		return nil, nil
	}
	if !s.chunksFTS5Enabled(ctx) {
		return s.searchChunksLike(ctx, rawQuery, fileIDs, limit)
	}
	query = buildFTSQuery(rawQuery)
	if query == "" {
		return nil, nil
	}
	where := `chunks_fts MATCH ? AND c.is_parent = 0 AND f.deleted_at IS NULL`
	args := []any{query}
	cleanFileIDs := cleanChunkFileIDs(fileIDs)
	if len(cleanFileIDs) > 0 {
		placeholders := make([]string, len(cleanFileIDs))
		for i, fileID := range cleanFileIDs {
			placeholders[i] = "?"
			args = append(args, fileID)
		}
		where += ` AND c.file_id IN (` + strings.Join(placeholders, ",") + `)`
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT
    c.id,
    c.file_id,
    c.file_name,
    c.heading,
    c.chunk_index,
    c.text,
    c.parent_chunk_id,
    bm25(chunks_fts) AS rank
FROM chunks_fts
JOIN chunks c ON c.rowid = chunks_fts.rowid
JOIN files f ON f.id = c.file_id
WHERE %s
ORDER BY rank ASC
LIMIT ?`, where), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sources := make([]model.SourceChunk, 0, limit)
	for rows.Next() {
		var source model.SourceChunk
		var rank float64
		if err := rows.Scan(
			&source.ID,
			&source.FileID,
			&source.FileName,
			&source.Heading,
			&source.ChunkIndex,
			&source.Text,
			&source.ParentID,
			&rank,
		); err != nil {
			return nil, err
		}
		source.Distance = float32(rank)
		source.Score = bm25RankScore(rank)
		source.Snippet = chunkSnippet(source.Text, defaultChunkSnippetLength)
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

func (s *Store) searchChunksLike(ctx context.Context, query string, fileIDs []string, limit int) ([]model.SourceChunk, error) {
	where := `c.is_parent = 0 AND f.deleted_at IS NULL AND c.text LIKE '%' || ? || '%' COLLATE NOCASE`
	args := []any{query}
	cleanFileIDs := cleanChunkFileIDs(fileIDs)
	if len(cleanFileIDs) > 0 {
		placeholders := make([]string, len(cleanFileIDs))
		for i, fileID := range cleanFileIDs {
			placeholders[i] = "?"
			args = append(args, fileID)
		}
		where += ` AND c.file_id IN (` + strings.Join(placeholders, ",") + `)`
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT
    c.id,
    c.file_id,
    c.file_name,
    c.heading,
    c.chunk_index,
    c.text,
    c.parent_chunk_id
FROM chunks c
JOIN files f ON f.id = c.file_id
WHERE %s
ORDER BY c.updated_at DESC
LIMIT ?`, where), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sources := make([]model.SourceChunk, 0, limit)
	for rows.Next() {
		var source model.SourceChunk
		if err := rows.Scan(
			&source.ID,
			&source.FileID,
			&source.FileName,
			&source.Heading,
			&source.ChunkIndex,
			&source.Text,
			&source.ParentID,
		); err != nil {
			return nil, err
		}
		source.Score = 1
		source.Snippet = chunkSnippet(source.Text, defaultChunkSnippetLength)
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

func (s *Store) chunksFTS5Enabled(ctx context.Context) bool {
	var schema string
	err := s.db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE name = 'chunks_fts'`).Scan(&schema)
	return err == nil && strings.Contains(strings.ToLower(schema), "using fts5")
}

func buildFTSQuery(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	terms := strings.FieldsFunc(query, func(r rune) bool {
		return unicode.IsSpace(r) || (unicode.IsPunct(r) && r != '_' && r != '-')
	})
	parts := []string{quoteFTS(query)}
	seen := map[string]struct{}{query: {}}
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		parts = append(parts, quoteFTS(term))
	}
	return strings.Join(parts, " OR ")
}

func quoteFTS(value string) string {
	value = strings.ReplaceAll(value, `"`, `""`)
	return `"` + value + `"`
}

func cleanChunkFileIDs(fileIDs []string) []string {
	cleaned := make([]string, 0, len(fileIDs))
	seen := map[string]struct{}{}
	for _, fileID := range fileIDs {
		fileID = strings.TrimSpace(fileID)
		if fileID == "" {
			continue
		}
		if _, ok := seen[fileID]; ok {
			continue
		}
		seen[fileID] = struct{}{}
		cleaned = append(cleaned, fileID)
	}
	return cleaned
}

func bm25RankScore(rank float64) float32 {
	if math.IsNaN(rank) || math.IsInf(rank, 0) {
		return 0
	}
	if rank < 0 {
		rank = -rank
	}
	return float32(1 / (1 + rank))
}

func chunkSnippet(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return strings.TrimSpace(string(runes[:limit])) + "..."
}
