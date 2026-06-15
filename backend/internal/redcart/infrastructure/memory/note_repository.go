package memory

import (
	"fmt"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"sort"
)

func (r *Repository) ListNotes() []domain.Note {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Note, 0, len(r.notes))
	for _, note := range r.notes {
		out = append(out, cloneNote(note))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *Repository) GetNote(id int64) (domain.Note, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	note, ok := r.notes[id]
	return cloneNote(note), ok
}

func (r *Repository) UpdateNote(note domain.Note) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.notes[note.ID]; !ok {
		return fmt.Errorf("note not found")
	}
	r.notes[note.ID] = cloneNote(note)
	return nil
}
