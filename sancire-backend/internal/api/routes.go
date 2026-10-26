package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"sancireai/backend/internal/config"
)

// corsMaxAge is how long a browser may cache a preflight answer.
const corsMaxAge = 300 * time.Second

// SetupRouter wires every HTTP endpoint of the service: middleware chain,
// public routes and the admin group protected by the DOU_Admin role.
func SetupRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	// Restrict credentialed cross-origin access to the configured UI origins
	// (SANCIRE_CORS_ORIGINS, local development ports by default).
	origins := h.cfg.CORSOrigins
	if len(origins) == 0 {
		origins = config.DefaultCORSOrigins
	}
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   origins,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link", "Content-Disposition", "Content-Length"},
		AllowCredentials: true,
		MaxAge:           int(corsMaxAge.Seconds()),
	}))
	r.Use(AuthMiddleware(h.cfg.JWTSecret))

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", h.handleHealth)

		// Auth
		r.Post("/auth/login", h.handleLogin)
		r.Get("/auth/me", h.handleMe)

		// Chat (JSON or SSE, chosen by the Accept header / ?stream=true)
		r.Post("/chat", h.handleChat)

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

		// Templates and the modular document catalogue
		r.Get("/templates", h.handleListTemplates)
		r.Get("/modules", h.handleListModules)

		// Regulations reader
		r.Get("/regulations", h.handleGetRegulations)

		// Document generation (level 2) and audit (level 3)
		r.Post("/documents/generate", h.handleGenerateDocument)
		r.Post("/documents/audit", h.handleAuditDocument)
		r.Get("/artifacts/{id}", h.handleGetArtifact)

		// Admin & versioning (RBAC: DOU_Admin only)
		r.Route("/admin", func(r chi.Router) {
			r.Get("/stats", RequireAdmin(h.handleAdminStats))
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
