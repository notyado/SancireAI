package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/bcrypt"

	_ "modernc.org/sqlite"
)

// Roles, defaults and seed data of the local SQLite store.
const (
	RoleEmployee = "Employee"
	RoleAdmin    = "DOU_Admin"

	DefaultTheme       = "dark"
	DefaultLanguage    = "ru"
	DefaultVersionCode = "v1.0"

	defaultSettingsID = "default"

	// Demo accounts created on an empty database (documented in README).
	demoEmployeePassword = "employee123"
	demoAdminPassword    = "admin123"

	// Edition of the seeded ОД-184 version row.
	od184EditionDate = "15.10.2026"
)

// User is an authenticated employee or DOU administrator.
type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"` // RoleEmployee or RoleAdmin
	Name         string    `json:"name"`
	CreatedAt    time.Time `json:"createdAt"`
}

// Conversation is one chat thread; Draft carries the wizard state.
type Conversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Pinned    bool      `json:"pinned"`
	Draft     any       `json:"draft"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Message is one chat turn (role: user | assistant).
type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversationId"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	Meta           any       `json:"meta"`
	CreatedAt      time.Time `json:"createdAt"`
}

// Template is a catalogue entry of the template library.
type Template struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Content     string `json:"content"`
	Popular     bool   `json:"popular"`
	Size        string `json:"size"`
}

// Setting holds the single UI preferences row (theme/language).
type Setting struct {
	ID       string `json:"id"`
	Theme    string `json:"theme"`
	Language string `json:"language"`
}

// Version is one edition of the normative base.
type Version struct {
	ID           string    `json:"id"`
	Code         string    `json:"code"`
	Title        string    `json:"title"`
	IsActive     bool      `json:"isActive"`
	Checksum     string    `json:"checksum"`
	IntroducedAt string    `json:"introducedAt"`
	Metadata     any       `json:"metadata"`
	CreatedAt    time.Time `json:"createdAt"`
}

// Store is the SQLite-backed persistence layer.
type Store struct {
	db *sql.DB
}

func New(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create data dir: %w", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db: %w", err)
	}
	// Enable WAL mode for concurrent speed and resilience
	if _, err := db.Exec("PRAGMA journal_mode = WAL; PRAGMA foreign_keys = ON;"); err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migration failed: %w", err)
	}
	if err := s.seed(); err != nil {
		return nil, fmt.Errorf("seed failed: %w", err)
	}
	return s, nil
}

// Close releases the database handle.
func (s *Store) Close() error {
	return s.db.Close()
}

// migrate creates every table if it does not exist yet.
func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'Employee',
		name TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS conversations (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		pinned BOOLEAN NOT NULL DEFAULT 0,
		draft TEXT,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS messages (
		id TEXT PRIMARY KEY,
		conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		meta TEXT,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS templates (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		category TEXT NOT NULL,
		description TEXT NOT NULL,
		content TEXT NOT NULL,
		popular BOOLEAN NOT NULL DEFAULT 0,
		size TEXT NOT NULL DEFAULT 'sm'
	);
	CREATE TABLE IF NOT EXISTS settings (
		id TEXT PRIMARY KEY,
		theme TEXT NOT NULL DEFAULT 'dark',
		language TEXT NOT NULL DEFAULT 'ru'
	);
	CREATE TABLE IF NOT EXISTS versions (
		id TEXT PRIMARY KEY,
		code TEXT UNIQUE NOT NULL,
		title TEXT NOT NULL,
		is_active BOOLEAN NOT NULL DEFAULT 0,
		checksum TEXT NOT NULL,
		introduced_at TEXT NOT NULL,
		metadata TEXT,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS regulations (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		kind TEXT NOT NULL DEFAULT 'Другое',
		filename TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'draft',
		file_path TEXT NOT NULL,
		chunk_count INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS regulation_chunks (
		id TEXT PRIMARY KEY,
		regulation_id TEXT NOT NULL REFERENCES regulations(id) ON DELETE CASCADE,
		seq INTEGER NOT NULL,
		clause TEXT NOT NULL,
		title TEXT NOT NULL,
		text TEXT NOT NULL,
		page INTEGER NOT NULL DEFAULT 0
	);
	CREATE TABLE IF NOT EXISTS artifacts (
		id TEXT PRIMARY KEY,
		filename TEXT NOT NULL,
		title TEXT NOT NULL DEFAULT '',
		kind TEXT NOT NULL DEFAULT '',
		preview TEXT NOT NULL DEFAULT '',
		data BLOB NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := s.db.Exec(schema)
	return err
}
// seed fills an empty database with the demo users, UI settings, the initial
// version row and the template catalogue. Every step is skipped when the
// corresponding table already has rows, so restarts never duplicate data.
func (s *Store) seed() error {
	if empty, err := s.tableEmpty("users"); err != nil {
		return err
	} else if empty {
		hashEmp, err := bcrypt.GenerateFromPassword([]byte(demoEmployeePassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		hashAdmin, err := bcrypt.GenerateFromPassword([]byte(demoAdminPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if _, err := s.db.Exec(`INSERT INTO users (id, username, password_hash, role, name) VALUES
			('usr-emp-1', 'employee', ?, ?, 'Сотрудник (Исполнитель)'),
			('usr-adm-1', 'admin', ?, ?, 'Администратор ДОУ')`,
			string(hashEmp), RoleEmployee, string(hashAdmin), RoleAdmin); err != nil {
			return err
		}
	}
	if empty, err := s.tableEmpty("settings"); err != nil {
		return err
	} else if empty {
		if _, err := s.db.Exec(`INSERT INTO settings (id, theme, language) VALUES (?, ?, ?)`,
			defaultSettingsID, DefaultTheme, DefaultLanguage); err != nil {
			return err
		}
	}
	if empty, err := s.tableEmpty("versions"); err != nil {
		return err
	} else if empty {
		metaJSON, err := json.Marshal(map[string]any{
			"document": "Инструкция по ДОУ (Приказ № ОД-184)",
			"editor":   "Центр документационного обеспечения Банка России",
			"status":   "Действующая",
		})
		if err != nil {
			return err
		}
		if _, err := s.db.Exec(`INSERT INTO versions (id, code, title, is_active, checksum, introduced_at, metadata) VALUES
			(?, ?, ?, 1, 'od184-sha256-verified', ?, ?)`,
			DefaultVersionCode, DefaultVersionCode,
			"Инструкция по ДОУ Банка России № ОД-184 (Редакция от "+od184EditionDate+")",
			od184EditionDate, string(metaJSON)); err != nil {
			return err
		}
	}
	if empty, err := s.tableEmpty("templates"); err != nil {
		return err
	} else if empty {
		return s.seedTemplates()
	}
	return nil
}

// tableEmpty reports whether a table has no rows yet.
func (s *Store) tableEmpty(table string) (bool, error) {
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
		return false, err
	}
	return count == 0, nil
}
func (s *Store) seedTemplates() error {
	tpls := []Template{
		{
			ID:          "memo",
			Title:       "Служебная записка",
			Category:    "Документооборот",
			Description: "Обращение к руководителю или смежному подразделению с предложением, запросом или обоснованием.",
			Popular:     true,
			Size:        "lg",
			Content:     "СЛУЖЕБНАЯ ЗАПИСКА\n\nКому: {{addressee}}\nТема: {{subject}}\n\n{{body}}",
		},
		{
			ID:          "trip",
			Title:       "Служебное задание на командировку",
			Category:    "Командировки",
			Description: "Основание для приказа о командировании: цель, сроки, место и ожидаемый результат.",
			Popular:     true,
			Size:        "lg",
			Content:     "СЛУЖЕБНОЕ ЗАДАНИЕ\nна командировку\n\nМесто: {{destination}}\nЦель: {{purpose}}",
		},
		{
			ID:          "leave",
			Title:       "Заявление на отпуск",
			Category:    "Кадры",
			Description: "Заявление о предоставлении ежегодного оплачиваемого отпуска вне графика или по графику.",
			Popular:     false,
			Size:        "sm",
			Content:     "ЗАЯВЛЕНИЕ\n\nПрошу предоставить ежегодный отпуск с {{dateFrom}} по {{dateTo}}.",
		},
		{
			ID:          "m01",
			Title:       "Приказ о вводе ИС в эксплуатацию (М-01)",
			Category:    "Модульные документы",
			Description: "Утвержденный модуль приказа о вводе информационной системы в эксплуатацию Банка.",
			Popular:     true,
			Size:        "lg",
			Content:     "ПРИКАЗ\nО вводе в эксплуатацию информационной системы {{is_name}}",
		},
		{
			ID:          "m02",
			Title:       "Приказ об ответственных за делопроизводство (М-02)",
			Category:    "Модульные документы",
			Description: "Назначение основного и замещающего работников за ведение делопроизводства в подразделении.",
			Popular:     false,
			Size:        "sm",
			Content:     "ПРИКАЗ\nО назначении ответственных за делопроизводство в {{department}}",
		},
		{
			ID:          "m03",
			Title:       "Приказ о приеме-передаче дел и имущества (М-03)",
			Category:    "Модульные документы",
			Description: "Передача дел, документов, ценностей и прав доступа к ИС при смене руководителя.",
			Popular:     true,
			Size:        "lg",
			Content:     "ПРИКАЗ\nО приеме и передаче дел и имущества",
		},
		{
			ID:          "m04",
			Title:       "Приказ об утверждении локального акта (М-04)",
			Category:    "Модульные документы",
			Description: "Введение в действие новых регламентов, стандартов и порядков Банка.",
			Popular:     false,
			Size:        "sm",
			Content:     "ПРИКАЗ\nОб утверждении и введении в действие {{act_name}}",
		},
		{
			ID:          "m05",
			Title:       "Распоряжение о внутреннем аудите процессов (М-05)",
			Category:    "Модульные документы",
			Description: "Плановый или внеплановый аудит контрольных процедур и процессов подразделения.",
			Popular:     false,
			Size:        "sm",
			Content:     "РАСПОРЯЖЕНИЕ\nО проведении внутреннего аудита процесса {{process_name}}",
		},
		{
			ID:          "m06",
			Title:       "Распоряжение о проектном комитете (М-06)",
			Category:    "Модульные документы",
			Description: "Создание коллегиального органа стратегического управления проектом или портфелем.",
			Popular:     false,
			Size:        "sm",
			Content:     "РАСПОРЯЖЕНИЕ\nО создании проектного комитета по проекту {{project_name}}",
		},
		{
			ID:          "m07",
			Title:       "Распоряжение о рабочей группе (М-07)",
			Category:    "Модульные документы",
			Description: "Создание временной межфункциональной группы для решения оперативной задачи.",
			Popular:     true,
			Size:        "lg",
			Content:     "РАСПОРЯЖЕНИЕ\nО создании рабочей группы",
		},
	}
	for _, tpl := range tpls {
		if _, err := s.db.Exec(`INSERT INTO templates (id, title, category, description, content, popular, size) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			tpl.ID, tpl.Title, tpl.Category, tpl.Description, tpl.Content, tpl.Popular, tpl.Size); err != nil {
			return err
		}
	}
	return nil
}

// GetUserByUsername loads a user by login.
func (s *Store) GetUserByUsername(username string) (*User, error) {
	row := s.db.QueryRow("SELECT id, username, password_hash, role, name, created_at FROM users WHERE username = ?", username)
	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Name, &u.CreatedAt); err != nil {
		return nil, err
	}
	return &u, nil
}

// ValidateUser checks the credentials. The messages are deliberately generic:
// they are surfaced to the client, which must not learn which half failed.
func (s *Store) ValidateUser(username, password string) (*User, error) {
	u, err := s.GetUserByUsername(username)
	if err != nil {
		return nil, errors.New("пользователь не найден")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, errors.New("неверный пароль")
	}
	return u, nil
}

// ListConversations returns every thread, pinned first.
func (s *Store) ListConversations() ([]Conversation, error) {
	rows, err := s.db.Query("SELECT id, title, pinned, draft, created_at, updated_at FROM conversations ORDER BY pinned DESC, updated_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []Conversation
	for rows.Next() {
		var c Conversation
		var draftStr sql.NullString
		if err := rows.Scan(&c.ID, &c.Title, &c.Pinned, &draftStr, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		if draftStr.Valid && draftStr.String != "" {
			_ = json.Unmarshal([]byte(draftStr.String), &c.Draft)
		}
		res = append(res, c)
	}
	if res == nil {
		res = []Conversation{}
	}
	return res, nil
}

// GetConversation loads one thread including its wizard draft.
func (s *Store) GetConversation(id string) (*Conversation, error) {
	row := s.db.QueryRow("SELECT id, title, pinned, draft, created_at, updated_at FROM conversations WHERE id = ?", id)
	var c Conversation
	var draftStr sql.NullString
	if err := row.Scan(&c.ID, &c.Title, &c.Pinned, &draftStr, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	if draftStr.Valid && draftStr.String != "" {
		_ = json.Unmarshal([]byte(draftStr.String), &c.Draft)
	}
	return &c, nil
}

// CreateConversation inserts a new empty thread.
func (s *Store) CreateConversation(id, title string) (*Conversation, error) {
	now := time.Now().UTC()
	_, err := s.db.Exec("INSERT INTO conversations (id, title, pinned, draft, created_at, updated_at) VALUES (?, ?, 0, NULL, ?, ?)",
		id, title, now, now)
	if err != nil {
		return nil, err
	}
	return &Conversation{
		ID:        id,
		Title:     title,
		Pinned:    false,
		Draft:     nil,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}
func (s *Store) UpdateConversation(id string, title *string, pinned *bool, draft any) (*Conversation, error) {
	c, err := s.GetConversation(id)
	if err != nil {
		return nil, err
	}
	newTitle := c.Title
	if title != nil {
		newTitle = *title
	}
	newPinned := c.Pinned
	if pinned != nil {
		newPinned = *pinned
	}
	// Draft semantics: nil keeps the existing draft (a title/pin update must
	// not wipe a wizard in progress); any non-nil value replaces it — pass
	// json.RawMessage("null") to clear it explicitly.
	srcDraft := draft
	if srcDraft == nil {
		srcDraft = c.Draft
	}
	var draftJSON *string
	if srcDraft != nil {
		b, err := json.Marshal(srcDraft)
		if err != nil {
			return nil, fmt.Errorf("marshal draft: %w", err)
		}
		raw := string(b)
		draftJSON = &raw
	}
	now := time.Now().UTC()
	_, err = s.db.Exec("UPDATE conversations SET title = ?, pinned = ?, draft = ?, updated_at = ? WHERE id = ?",
		newTitle, newPinned, draftJSON, now, id)
	if err != nil {
		return nil, err
	}
	return s.GetConversation(id)
}

// DeleteConversation removes a thread; its messages follow via ON DELETE CASCADE.
func (s *Store) DeleteConversation(id string) error {
	_, err := s.db.Exec("DELETE FROM conversations WHERE id = ?", id)
	return err
}

// ClearAllConversations wipes every thread («Очистить историю» in the sidebar).
func (s *Store) ClearAllConversations() error {
	_, err := s.db.Exec("DELETE FROM conversations")
	return err
}

// ListMessages returns the turns of a thread in chronological order.
func (s *Store) ListMessages(conversationID string) ([]Message, error) {
	rows, err := s.db.Query("SELECT id, conversation_id, role, content, meta, created_at FROM messages WHERE conversation_id = ? ORDER BY created_at ASC, rowid ASC", conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []Message
	for rows.Next() {
		var m Message
		var metaStr sql.NullString
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &metaStr, &m.CreatedAt); err != nil {
			return nil, err
		}
		if metaStr.Valid && metaStr.String != "" {
			_ = json.Unmarshal([]byte(metaStr.String), &m.Meta)
		}
		res = append(res, m)
	}
	if res == nil {
		res = []Message{}
	}
	return res, nil
}
// CreateMessage stores one turn and bumps the thread's updated_at.
func (s *Store) CreateMessage(id, conversationID, role, content string, meta any) (*Message, error) {
	var metaJSON *string
	if meta != nil {
		b, err := json.Marshal(meta)
		if err != nil {
			return nil, fmt.Errorf("marshal message meta: %w", err)
		}
		raw := string(b)
		metaJSON = &raw
	}
	now := time.Now().UTC()
	_, err := s.db.Exec("INSERT INTO messages (id, conversation_id, role, content, meta, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		id, conversationID, role, content, metaJSON, now)
	if err != nil {
		return nil, err
	}
	if _, err := s.db.Exec("UPDATE conversations SET updated_at = ? WHERE id = ?", now, conversationID); err != nil {
		return nil, err
	}
	return &Message{
		ID:             id,
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
		Meta:           meta,
		CreatedAt:      now,
	}, nil
}

// ListTemplates returns the seeded template catalogue.
func (s *Store) ListTemplates() ([]Template, error) {
	rows, err := s.db.Query("SELECT id, title, category, description, content, popular, size FROM templates ORDER BY title ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []Template
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.Title, &t.Category, &t.Description, &t.Content, &t.Popular, &t.Size); err != nil {
			return nil, err
		}
		res = append(res, t)
	}
	return res, nil
}

// GetSettings returns the UI preferences, falling back to the defaults.
func (s *Store) GetSettings() (*Setting, error) {
	row := s.db.QueryRow("SELECT id, theme, language FROM settings WHERE id = ?", defaultSettingsID)
	var st Setting
	if err := row.Scan(&st.ID, &st.Theme, &st.Language); err != nil {
		return &Setting{ID: defaultSettingsID, Theme: DefaultTheme, Language: DefaultLanguage}, nil
	}
	return &st, nil
}

// UpdateSettings upserts the single preferences row.
func (s *Store) UpdateSettings(theme, language string) (*Setting, error) {
	_, err := s.db.Exec(`INSERT INTO settings (id, theme, language) VALUES (?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET theme = excluded.theme, language = excluded.language`,
		defaultSettingsID, theme, language)
	if err != nil {
		return nil, err
	}
	return s.GetSettings()
}

// ListVersions returns every edition, newest first.
func (s *Store) ListVersions() ([]Version, error) {
	rows, err := s.db.Query("SELECT id, code, title, is_active, checksum, introduced_at, metadata, created_at FROM versions ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []Version
	for rows.Next() {
		var v Version
		var metaStr sql.NullString
		if err := rows.Scan(&v.ID, &v.Code, &v.Title, &v.IsActive, &v.Checksum, &v.IntroducedAt, &metaStr, &v.CreatedAt); err != nil {
			return nil, err
		}
		if metaStr.Valid && metaStr.String != "" {
			_ = json.Unmarshal([]byte(metaStr.String), &v.Metadata)
		}
		res = append(res, v)
	}
	return res, nil
}
// ActivateVersion makes one edition active; exactly one row stays active.
func (s *Store) ActivateVersion(code string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec("UPDATE versions SET is_active = 0"); err != nil {
		return err
	}
	res, err := tx.Exec("UPDATE versions SET is_active = 1 WHERE code = ?", code)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("версия с кодом %s не найдена", code)
	}
	return tx.Commit()
}

// GetActiveVersion returns the currently active edition.
func (s *Store) GetActiveVersion() (*Version, error) {
	row := s.db.QueryRow("SELECT id, code, title, is_active, checksum, introduced_at, metadata, created_at FROM versions WHERE is_active = 1 LIMIT 1")
	var v Version
	var metaStr sql.NullString
	if err := row.Scan(&v.ID, &v.Code, &v.Title, &v.IsActive, &v.Checksum, &v.IntroducedAt, &metaStr, &v.CreatedAt); err != nil {
		return nil, err
	}
	if metaStr.Valid && metaStr.String != "" {
		_ = json.Unmarshal([]byte(metaStr.String), &v.Metadata)
	}
	return &v, nil
}

// Artifact is a generated .docx file served back to the chat client.
type Artifact struct {
	ID       string    `json:"id"`
	Filename string    `json:"filename"`
	Title    string    `json:"title"`
	Kind     string    `json:"kind"`
	Preview  string    `json:"preview"`
	Data     []byte    `json:"-"`
	CreatedAt time.Time `json:"createdAt"`
}

// SaveArtifact persists a generated document.
func (s *Store) SaveArtifact(id, filename, title, kind, preview string, data []byte) error {
	_, err := s.db.Exec(
		"INSERT INTO artifacts (id, filename, title, kind, preview, data) VALUES (?, ?, ?, ?, ?, ?)",
		id, filename, title, kind, preview, data)
	return err
}

// GetArtifact loads a generated document by id.
func (s *Store) GetArtifact(id string) (*Artifact, error) {
	row := s.db.QueryRow("SELECT id, filename, title, kind, preview, data, created_at FROM artifacts WHERE id = ?", id)
	var a Artifact
	if err := row.Scan(&a.ID, &a.Filename, &a.Title, &a.Kind, &a.Preview, &a.Data, &a.CreatedAt); err != nil {
		return nil, err
	}
	return &a, nil
}

// ---------------------------------------------------------------------------
// Regulations (admin-managed normative documents)
// ---------------------------------------------------------------------------

// Regulation statuses.
const (
	RegDraft    = "draft"
	RegActive   = "active"
	RegArchived = "archived"
)

// Regulation is an uploaded normative document under admin control.
type Regulation struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Kind       string    `json:"kind"`
	Filename   string    `json:"filename"`
	Status     string    `json:"status"`
	ChunkCount int       `json:"chunkCount"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// RegulationChunk is one indexed piece of a regulation (joined with its
// document metadata for direct use by the knowledge index).
type RegulationChunk struct {
	ID            string
	RegulationID  string
	Seq           int
	Clause        string
	Title         string
	Text          string
	Page          int
	DocumentTitle string
	Kind          string
	Status        string
	UpdatedAt     string
}

// SaveRegulation persists a document together with its chunks (one tx).
func (s *Store) SaveRegulation(id, title, kind, filename, filePath, status string, chunks []RegulationChunk) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	_, err = tx.Exec(
		`INSERT INTO regulations (id, title, kind, filename, status, file_path, chunk_count, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, title, kind, filename, status, filePath, len(chunks), now, now)
	if err != nil {
		return err
	}
	for _, c := range chunks {
		_, err = tx.Exec(
			`INSERT INTO regulation_chunks (id, regulation_id, seq, clause, title, text, page) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			c.ID, id, c.Seq, c.Clause, c.Title, c.Text, c.Page)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListRegulations returns all documents, newest first.
func (s *Store) ListRegulations() ([]Regulation, error) {
	rows, err := s.db.Query(
		`SELECT id, title, kind, filename, status, chunk_count, created_at, updated_at
		 FROM regulations ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Regulation
	for rows.Next() {
		var r Regulation
		if err := rows.Scan(&r.ID, &r.Title, &r.Kind, &r.Filename, &r.Status, &r.ChunkCount, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetRegulation loads one document by id.
func (s *Store) GetRegulation(id string) (*Regulation, error) {
	row := s.db.QueryRow(
		`SELECT id, title, kind, filename, status, chunk_count, created_at, updated_at
		 FROM regulations WHERE id = ?`, id)
	var r Regulation
	if err := row.Scan(&r.ID, &r.Title, &r.Kind, &r.Filename, &r.Status, &r.ChunkCount, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	return &r, nil
}

// RegulationFilePath returns the stored original file path.
func (s *Store) RegulationFilePath(id string) (string, error) {
	var p string
	err := s.db.QueryRow(`SELECT file_path FROM regulations WHERE id = ?`, id).Scan(&p)
	return p, err
}

// SetRegulationStatus switches a document status. When activating, any other
// active document of the SAME kind is demoted to archived (one active per
// kind at a time). Returns the ids of chunks that lost/changed index state:
// prevActiveIDs (chunks of the document that was just deactivated, if any)
// and the target's own chunk ids when the new status is active.
func (s *Store) SetRegulationStatus(id, status string) (prevActiveIDs []string, targetIDs []string, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var kind string
	if err = tx.QueryRow(`SELECT kind FROM regulations WHERE id = ?`, id).Scan(&kind); err != nil {
		return nil, nil, err
	}
	if status == RegActive {
		rows, e := tx.Query(`SELECT id FROM regulations WHERE status = ? AND kind = ? AND id <> ?`, RegActive, kind, id)
		if e != nil {
			return nil, nil, e
		}
		var demote []string
		for rows.Next() {
			var other string
			if e := rows.Scan(&other); e != nil {
				rows.Close()
				return nil, nil, e
			}
			demote = append(demote, other)
		}
		rows.Close()
		for _, other := range demote {
			ids, e := s.chunkIDsTx(tx, other)
			if e != nil {
				return nil, nil, e
			}
			prevActiveIDs = append(prevActiveIDs, ids...)
			if _, e = tx.Exec(`UPDATE regulations SET status = ?, updated_at = ? WHERE id = ?`, RegArchived, time.Now().UTC(), other); e != nil {
				return nil, nil, e
			}
		}
	}
	if _, err = tx.Exec(`UPDATE regulations SET status = ?, updated_at = ? WHERE id = ?`, status, time.Now().UTC(), id); err != nil {
		return nil, nil, err
	}
	if status == RegActive {
		targetIDs, err = s.chunkIDsTx(tx, id)
		if err != nil {
			return nil, nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, nil, err
	}
	return prevActiveIDs, targetIDs, nil
}

// chunkIDsTx lists the chunk ids of one regulation inside a transaction.
func (s *Store) chunkIDsTx(tx *sql.Tx, regulationID string) ([]string, error) {
	rows, err := tx.Query(`SELECT id FROM regulation_chunks WHERE regulation_id = ?`, regulationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var cid string
		if err := rows.Scan(&cid); err != nil {
			return nil, err
		}
		ids = append(ids, cid)
	}
	return ids, rows.Err()
}

// RegulationChunkIDs lists chunk ids of a document (for vector cleanup).
func (s *Store) RegulationChunkIDs(id string) ([]string, error) {
	rows, err := s.db.Query(`SELECT id FROM regulation_chunks WHERE regulation_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var cid string
		if err := rows.Scan(&cid); err != nil {
			return nil, err
		}
		out = append(out, cid)
	}
	return out, rows.Err()
}

// loadRegulationChunks is the shared reader for chunk lists.
func (s *Store) loadRegulationChunks(where string, args ...any) ([]RegulationChunk, error) {
	rows, err := s.db.Query(`
		SELECT c.id, c.regulation_id, c.seq, c.clause, c.title, c.text, c.page,
		       r.title, r.kind, r.status, r.updated_at
		FROM regulation_chunks c
		JOIN regulations r ON r.id = c.regulation_id
		WHERE `+where+` ORDER BY r.created_at DESC, c.seq ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RegulationChunk
	for rows.Next() {
		var c RegulationChunk
		var updated time.Time
		if err := rows.Scan(&c.ID, &c.RegulationID, &c.Seq, &c.Clause, &c.Title, &c.Text, &c.Page,
			&c.DocumentTitle, &c.Kind, &c.Status, &updated); err != nil {
			return nil, err
		}
		c.UpdatedAt = updated.Format("02.01.2006")
		out = append(out, c)
	}
	return out, rows.Err()
}

// ActiveRegulationChunks returns chunks of every ACTIVE regulation — the
// slice that must be searchable in chat.
func (s *Store) ActiveRegulationChunks() ([]RegulationChunk, error) {
	return s.loadRegulationChunks(`r.status = ?`, RegActive)
}

// RegulationChunks returns all chunks of one document.
func (s *Store) RegulationChunks(id string) ([]RegulationChunk, error) {
	return s.loadRegulationChunks(`c.regulation_id = ?`, id)
}

// DeleteRegulation removes a document and its chunks (file removal is the
// caller's job).
func (s *Store) DeleteRegulation(id string) error {
	_, err := s.db.Exec(`DELETE FROM regulations WHERE id = ?`, id)
	return err
}
