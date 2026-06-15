package postgres

import (
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

func (r *Repository) ListNotes() []domain.Note {
	rows, err := r.db.Query(`SELECT id, author_id, title, content, cover_url, status, view_count, like_count, created_at, updated_at FROM notes ORDER BY id`)
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
		note.ProductIDs = r.loadNoteProductIDs(note.ID)
		notes = append(notes, note)
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
