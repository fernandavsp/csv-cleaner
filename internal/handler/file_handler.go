package handler

import (
	"csv-cleaner/internal/database"
	"csv-cleaner/internal/service"
	"csv-cleaner/internal/utils"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var MaxFileSizeMB int64 = 10

func UploadFile(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "arquivo inválido"})
		return
	}

	if file.Size > MaxFileSizeMB*1024*1024 {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"error": "arquivo muito grande, limite de " + strconv.FormatInt(MaxFileSizeMB, 10) + "MB",
		})
		return
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".csv" && ext != ".tsv" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "apenas arquivos .csv e .tsv são aceitos"})
		return
	}

	f, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao abrir o arquivo"})
		return
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	contentType := http.DetectContentType(buf[:n])
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar o arquivo"})
		return
	}

	if !strings.HasPrefix(contentType, "text/") && contentType != "application/octet-stream" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "conteúdo do arquivo inválido"})
		return
	}

	data, err := service.ParseCSV(f)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "erro ao interpretar CSV: " + err.Error()})
		return
	}

	id := uuid.New().String()

	jsonData, _ := json.Marshal(data)

	_, err = database.DB.Exec(
		"INSERT INTO files (id, original_name, data) VALUES ($1, $2, $3)",
		id,
		file.Filename,
		string(jsonData),
	)
	if err != nil {
		slog.Error("erro ao salvar arquivo", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao salvar o arquivo"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"file_id": id,
		"preview": data,
	})
}

func ProcessFile(c *gin.Context) {
	id := c.Query("file_id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file_id é obrigatório"})
		return
	}

	dateFormat := strings.ToLower(strings.TrimSpace(c.DefaultQuery("date_format", service.DateFormatNoChange)))
	if dateFormat != service.DateFormatNoChange &&
		dateFormat != service.DateFormatISO &&
		dateFormat != service.DateFormatBR &&
		dateFormat != service.DateFormatUS {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date_format inválido. Use: none, iso, br ou us"})
		return
	}

	var jsonData string

	err := database.DB.QueryRow(
		"SELECT data FROM files WHERE id=$1",
		id,
	).Scan(&jsonData)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "arquivo não encontrado"})
		return
	}

	var data [][]string
	json.Unmarshal([]byte(jsonData), &data)

	result := service.CleanData(data, dateFormat)

	updatedJSON, _ := json.Marshal(result.Data)

	_, err = database.DB.Exec(
		"UPDATE files SET data=$1 WHERE id=$2",
		string(updatedJSON),
		id,
	)
	if err != nil {
		slog.Error("erro ao atualizar arquivo", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao salvar resultado"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "arquivo processado",
		"rows_before":   len(data),
		"rows_after":    len(result.Data),
		"rows_removed":  result.RemovedRows,
		"cells_trimmed": result.TrimmedCells,
		"empty_removed": result.EmptyRowsRemoved,
		"dates_changed": result.DateFormatted,
		"dates_invalid": result.DateInvalid,
		"dates_checked": result.DateChecked,
		"date_format":   dateFormat,
		"preview":       result.Data,
	})
}

func DownloadFile(c *gin.Context) {
	id := c.Query("file_id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file_id é obrigatório"})
		return
	}

	var jsonData string

	err := database.DB.QueryRow(
		"SELECT data FROM files WHERE id=$1",
		id,
	).Scan(&jsonData)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "arquivo não encontrado"})
		return
	}

	var data [][]string
	json.Unmarshal([]byte(jsonData), &data)

	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	c.Header("Content-Disposition", "attachment; filename=cleaned.csv")
	c.Header("Content-Type", "text/csv")

	utils.WriteCSV(c.Writer, data)
}
