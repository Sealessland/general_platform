package postgres

import (
	"fmt"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

func (r *Repository) ListNotes(limit, offset int) []domain.Note {
	query := `SELECT id, author_id, title, content, cover_url, status, view_count, like_count, created_at, updated_at FROM notes ORDER BY id`
	if limit > 0 {
		query = fmt.Sprintf("%s LIMIT %d OFFSET %d", query, limit, offset)
	}
	rows, err := r.db.Query(query)
	if err != nil {
		return nil
	}
	defer rows.Close()
	notes := make([]domain.Note, 0)
	for rows.Next() {
		var note domain.Note
		if err := rows.Scan(&note.ID, &note.AuthorID, &note.Title, &note.Content, &note.CoverURL, &note.Status, &note.ViewCount, &note.LikeCount, &note.CreatedAt, &note.UpdatedAt); err != nil {
			return notes
		}
		notes = append(notes, note)
	}
	if len(notes) == 0 {
		return notes
	}

	// Batch load product IDs for all notes in one query (eliminates N+1).
	noteIDs := make([]int64, len(notes))
	for i := range notes {
		noteIDs[i] = notes[i].ID
	}
	productsByNote := r.loadNoteProductIDsBatch(noteIDs)
	for i := range notes {
		notes[i].ProductIDs = productsByNote[notes[i].ID]
	}
	return notes
}

func (r *Repository) GetNote(id int64) (domain.Note, bool) {
	row := r.db.QueryRow(`SELECT id, author_id, title, content, cover_url, status, view_count, like_count, created_at, updated_at FROM notes WHERE id = $1`, id)
	var note domain.Note
	if err := row.Scan(&note.ID, &note.AuthorID, &note.Title, &note.Content, &note.CoverURL, &note.Status, &note.ViewCount, &note.LikeCount, &note.CreatedAt, &note.UpdatedAt); err != nil {
		return domain.Note{}, false
	}
	note.ProductIDs = r.loadNoteProductIDs(note.ID)
	return note, true
}

func (r *Repository) UpdateNote(note domain.Note) error {
	_, err := r.db.Exec(
		`UPDATE notes SET author_id = $1, title = $2, content = $3, cover_url = $4, status = $5, view_count = $6, like_count = $7 WHERE id = $8`,
		note.AuthorID, note.Title, note.Content, note.CoverURL, note.Status, note.ViewCount, note.LikeCount, note.ID,
	)
	return err
}
func (r *Repository) loadNoteProductIDs(noteID int64) []int64 {
	rows, err := r.db.Query(`SELECT product_id FROM note_products WHERE note_id = $1 ORDER BY id`, noteID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]int64, 0)
	for rows.Next() {
		var productID int64
		if err := rows.Scan(&productID); err != nil {
			return out
		}
		out = append(out, productID)
	}
	return out
}

// loadNoteProductIDsBatch fetches product IDs for multiple notes in a single
// query, eliminating the N+1 problem where ListNotes called loadNoteProductIDs
// once per note. Returns a map keyed by note_id.
func (r *Repository) loadNoteProductIDsBatch(noteIDs []int64) map[int64][]int64 {
	if len(noteIDs) == 0 {
		return nil
	}
	rows, err := r.db.Query(
		`SELECT note_id, product_id FROM note_products WHERE note_id = ANY($1::bigint[]) ORDER BY id`,
		noteIDs,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	result := make(map[int64][]int64)
	for rows.Next() {
		var noteID, productID int64
		if err := rows.Scan(&noteID, &productID); err != nil {
			return result
		}
		result[noteID] = append(result[noteID], productID)
	}
	return result
}
