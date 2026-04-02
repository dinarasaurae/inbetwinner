package handlers

import (
	"io"
	"strconv"

	"github.com/dinarasaurae/inbetwin-rag-service/internal/models"
	"github.com/dinarasaurae/inbetwin-rag-service/internal/services"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type DocumentHandler struct{ svc *services.DocumentService }

func NewDocumentHandler(svc *services.DocumentService) *DocumentHandler {
	return &DocumentHandler{svc: svc}
}

// Create ingests plain-text content via JSON body.
// POST /rag/documents
// Body: { "namespace_id": "...", "filename": "...", "content_type": "text/plain", "content": "...", "chunk_size": 512 }
func (h *DocumentHandler) Create(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	var req models.CreateDocumentRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	doc, err := h.svc.Create(c.Context(), wid, req)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(doc)
}

// Upload handles multipart file uploads for PDF, DOCX, XLSX and plain text.
// POST /rag/documents/upload
// Form fields:
//   - file        — required, the binary file
//   - namespace_id — required UUID
//   - chunk_size  — optional int (default 512)
func (h *DocumentHandler) Upload(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)

	nsIDStr := c.FormValue("namespace_id")
	nsID, err := uuid.Parse(nsIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "namespace_id is required and must be a valid UUID"})
	}

	chunkSize := 512
	if cs := c.FormValue("chunk_size"); cs != "" {
		if n, err := strconv.Atoi(cs); err == nil && n > 0 {
			chunkSize = n
		}
	}

	fh, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "file field is required"})
	}

	// Guard: 50 MB limit
	const maxBytes = 50 << 20
	if fh.Size > maxBytes {
		return c.Status(413).JSON(fiber.Map{"error": "file too large (max 50 MB)"})
	}

	f, err := fh.Open()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "cannot open uploaded file"})
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "cannot read uploaded file"})
	}

	contentType := fh.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	doc, err := h.svc.Upload(c.Context(), wid, fh.Filename, contentType, data, nsID, chunkSize)
	if err != nil {
		return c.Status(422).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(doc)
}

func (h *DocumentHandler) List(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	nsIDStr := c.Query("namespace_id")
	nsID, err := uuid.Parse(nsIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid namespace_id"})
	}
	docs, err := h.svc.List(c.Context(), wid, nsID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if docs == nil {
		docs = []models.KnowledgeDocument{}
	}
	return c.JSON(docs)
}

func (h *DocumentHandler) Delete(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.svc.Delete(c.Context(), id, wid); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "deleted"})
}
