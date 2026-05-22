package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
)

type Camera struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

type CameraStore struct {
	sync.RWMutex
	cameras map[string]Camera
}

func (s *CameraStore) Add(c Camera) {
	s.Lock()
	defer s.Unlock()
	s.cameras[c.ID] = c
}

func (s *CameraStore) List() []Camera {
	s.RLock()
	defer s.RUnlock()
	list := make([]Camera, 0, len(s.cameras))
	for _, c := range s.cameras {
		list = append(list, c)
	}
	return list
}

func main() {
	store := &CameraStore{
		cameras: make(map[string]Camera),
	}

	http.HandleFunc("/cameras", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var c Camera
			if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			store.Add(c)
			w.WriteHeader(http.StatusCreated)
			return
		}

		json.NewEncoder(w).Encode(store.List())
	})

	log.Println("Camera Management Service listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
