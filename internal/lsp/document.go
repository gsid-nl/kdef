package lsp

import (
	"sync"
)

// Document represents an open file in the editor.
type Document struct {
	URI     string
	Content string
	Version int32
}

// DocumentStore tracks all open documents.
type DocumentStore struct {
	mu   sync.RWMutex
	docs map[string]*Document
}

// NewDocumentStore creates a new document store.
func NewDocumentStore() *DocumentStore {
	return &DocumentStore{
		docs: make(map[string]*Document),
	}
}

// Open stores a newly opened document.
func (s *DocumentStore) Open(uri, content string, version int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs[uri] = &Document{
		URI:     uri,
		Content: content,
		Version: version,
	}
}

// Update replaces the content of an open document.
func (s *DocumentStore) Update(uri, content string, version int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if doc, ok := s.docs[uri]; ok {
		doc.Content = content
		doc.Version = version
	}
}

// Close removes a document from the store.
func (s *DocumentStore) Close(uri string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.docs, uri)
}

// Get returns a document by URI, or nil if not found.
func (s *DocumentStore) Get(uri string) *Document {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.docs[uri]
}

// GetAllInDir returns all open documents whose URI starts with the given directory prefix.
func (s *DocumentStore) GetAllInDir(dirURI string) []*Document {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Document
	for _, doc := range s.docs {
		if len(doc.URI) >= len(dirURI) && doc.URI[:len(dirURI)] == dirURI {
			result = append(result, doc)
		}
	}
	return result
}
