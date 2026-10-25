package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"net/http"
)

func SetupRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	// Restrict credentialed cross-origin access to the local development UI origins.
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000", "http://127.0.0.1:3000", "http://localhost:5173", "http://127.0.0.1:5173"},
		AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-User-Role"},
		ExposedHeaders:   []string{"Link", "Content-Disposition", "Content-Length"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.Use(AuthMiddleware(h.cfg.JWTSecret))
	r.Route("/api", func(r chi.Router) {
		r.Get("/health", h.handleHealth)
		// Auth
		r.Post("/auth/login", h.handleLogin)
		r.Get("/auth/me", h.handleMe)
		// Chat
		r.Post("/chat", h.handleChat)
		r.Post("/chat/stream", h.handleChat) // dedicated stream endpoint alias
		// Conversations
		r.Get("/conversations", h.handleListConversations)
		r.Post("/conversations", h.handleCreateConversation)
		r.Delete("/conversations", h.handleDeleteAllConversations)
		r.Get("/conversations/{id}", h.handleGetConversation)
		r.Patch("/conversations/{id}", h.handleUpdateConversation)
		r.Delete("/conversations/{id}", h.handleDeleteConversation)
		// Settings
		r.Get("/settings", h.handleGetSettings)
		r.Patch("/settings", h.handleUpdateSettings)
		// Templates
		r.Get("/templates", h.handleListTemplates)
		// Regulations
		r.Get("/regulations", h.handleGetRegulations)
		// Level 2 & Level 3 Document Operations
		r.Post("/documents/generate", h.handleGenerateDocument)
		r.Post("/documents/audit", h.handleAuditDocument)
		r.Get("/artifacts/{id}", h.handleGetArtifact)
		// Admin & Versioning (RBAC protected: requires DOU_Admin role)
		r.Route("/admin", func(r chi.Router) {
			r.Get("/versions", RequireAdmin(h.handleListVersions))
			r.Post("/versions/activate", RequireAdmin(h.handleActivateVersion))
			// Regulation lifecycle: upload → activate → archive → delete
			r.Get("/regulations", RequireAdmin(h.handleListRegulations))
			r.Post("/regulations/upload", RequireAdmin(h.handleUploadRegulation))
			r.Post("/regulations/{id}/activate", RequireAdmin(h.handleActivateRegulation))
			r.Post("/regulations/{id}/archive", RequireAdmin(h.handleArchiveRegulation))
			r.Delete("/regulations/{id}", RequireAdmin(h.handleDeleteRegulation))
			r.Get("/regulations/{id}/file", RequireAdmin(h.handleDownloadRegulation))
		})
	})
	return r
}
