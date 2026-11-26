package studio

import "net/http"

type sessionDeletion struct {
	Deleted []string               `json:"deleted"`
	Failed  []sessionDeletionError `json:"failed"`
}

type sessionDeletionError struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

func (a *App) deleteSessions(w http.ResponseWriter, r *http.Request) {
	var input struct {
		IDs []string `json:"ids"`
	}
	if !decodeRequest(w, r, &input) {
		return
	}
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	result := sessionDeletion{Deleted: []string{}, Failed: []sessionDeletionError{}}
	for _, id := range input.IDs {
		// A generation writes original images outside the store lock. Let it finish
		// or stop before removing its directory; uploads already hold this lock.
		if _, running := a.jobs[id]; running {
			result.Failed = append(result.Failed, sessionDeletionError{id, "任务仍在生成，请先停止生成后再删除"})
			continue
		}
		if err := a.store.deleteSession(id); err != nil {
			result.Failed = append(result.Failed, sessionDeletionError{id, publicError(err, "").Message})
			continue
		}
		result.Deleted = append(result.Deleted, id)
	}
	writeResponse(w, http.StatusOK, result)
}
