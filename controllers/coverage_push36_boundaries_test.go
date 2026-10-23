package controllers

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// readAwardImport 批次边界：0 行 / 5001 行拒绝、5000 行放行
func TestReadAwardImport_BatchSizeBoundaries(t *testing.T) {
	gin.SetMode(gin.TestMode)

	makeCtx := func(rows []AwardImportRow) *gin.Context {
		payload, _ := json.Marshal(map[string]interface{}{"rows": rows})
		req := httptest.NewRequest("POST", "/x?comp_id=1", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		return c
	}

	// 0 行拒绝
	_, _, err := readAwardImport(makeCtx([]AwardImportRow{}))
	assert.ErrorContains(t, err, "每批请提交1至5000行")

	// 5001 行拒绝（超上限 1 行）
	over := make([]AwardImportRow, 5001)
	for i := range over {
		over[i] = importRow("S001", "项目A")
	}
	_, _, err = readAwardImport(makeCtx(over))
	assert.ErrorContains(t, err, "每批请提交1至5000行")

	// 5000 行（上限本身）放行
	exact := make([]AwardImportRow, 5000)
	for i := range exact {
		exact[i] = importRow("S001", "项目A")
	}
	_, data, err := readAwardImport(makeCtx(exact))
	assert.NoError(t, err)
	assert.Equal(t, 5000, len(data))
}

// 非 JSON 且无上传文件 → 文件上传失败
func TestReadAwardImport_MissingFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest("POST", "/x?comp_id=1", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	_, _, err := readAwardImport(c)
	assert.ErrorContains(t, err, "文件上传失败")
}
