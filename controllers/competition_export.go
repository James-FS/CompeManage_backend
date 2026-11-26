package controllers

import (
	"CompeManage_backend/utils"
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

const (
	competitionExportSheet = "赛事列表"
	// Keep each worksheet ZIP entry below the ZIP64 threshold, even at maximum field widths.
	competitionExportSheetRows = 100_000
	competitionExportTimeout   = 2 * time.Minute
)

func ExportCompetitionDirectory(c *gin.Context) {
	var req CompListReq
	if err := c.ShouldBindQuery(&req.CompetitionFilters); err != nil {
		utils.BadRequest(c, "参数错误")
		return
	}
	query, ok := competitionDirectoryQuery(c, req)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), competitionExportTimeout)
	defer cancel()
	workbook := excelize.NewFile()
	defer workbook.Close()
	count, err := writeCompetitionDirectory(workbook, query.WithContext(ctx))
	if err != nil {
		respondCompetitionExportError(c, ctx, err)
		return
	}
	if count == 0 {
		utils.SuccessWithMessage(c, "暂无数据可导出", nil)
		return
	}

	// Finish the archive on disk before sending headers so a failed export returns JSON.
	file, err := os.CreateTemp("", "competition-directory-*.xlsx")
	if err != nil {
		respondCompetitionExportError(c, ctx, err)
		return
	}
	defer os.Remove(file.Name())
	defer file.Close()
	// Excelize normally collects the compressed ZIP in memory. Its custom ZIP
	// writer can write the archive directly to the temporary file instead.
	workbook.SetZipWriter(func(io.Writer) excelize.ZipWriter {
		return competitionExportZip{Writer: zip.NewWriter(competitionExportWriter{ctx: ctx, writer: file}), ctx: ctx}
	})
	if err := workbook.Write(io.Discard); err != nil {
		respondCompetitionExportError(c, ctx, err)
		return
	}
	size, err := file.Seek(0, io.SeekCurrent)
	if err == nil {
		_, err = file.Seek(0, io.SeekStart)
	}
	if err != nil {
		respondCompetitionExportError(c, ctx, err)
		return
	}
	if err := ctx.Err(); err != nil {
		respondCompetitionExportError(c, ctx, err)
		return
	}
	filename := "赛事列表.xlsx"
	if req.Year != "" {
		filename = fmt.Sprintf("赛事列表_%s年度.xlsx", req.Year)
	}
	c.Header("Content-Disposition", `attachment; filename="competition-directory.xlsx"; filename*=UTF-8''`+url.PathEscape(filename))
	c.DataFromReader(http.StatusOK, size, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", file, nil)
}

type competitionExportWriter struct {
	ctx    context.Context
	writer io.Writer
}

type competitionExportZip struct {
	*zip.Writer
	ctx context.Context
}

func (w competitionExportZip) Create(name string) (io.Writer, error) {
	entry, err := w.Writer.Create(name)
	if err != nil {
		return nil, err
	}
	return competitionExportWriter{ctx: w.ctx, writer: entry}, nil
}

func (w competitionExportWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.writer.Write(p)
}

func respondCompetitionExportError(c *gin.Context, ctx context.Context, err error) {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		utils.Error(c, http.StatusGatewayTimeout, http.StatusGatewayTimeout, "赛事目录导出超时，请缩小筛选范围后重试")
		return
	}
	if c.Request.Context().Err() == nil {
		utils.InternalServerError(c, "生成赛事目录失败", err)
	}
}

func writeCompetitionDirectory(workbook *excelize.File, query *gorm.DB) (int, error) {
	rows, err := query.Joins("LEFT JOIN users export_manager ON export_manager.id = comp_directories.manager_id AND export_manager.delete_time IS NULL").
		Select(`COALESCE(comp_directories.comp_code, ''), COALESCE(comp_directories.comp_name, ''),
			COALESCE(comp_directories.comp_level, ''), COALESCE(comp_directories.organizer, ''),
			COALESCE(comp_directories.undertaker, ''), COALESCE(export_manager.realname, ''),
			COALESCE(export_manager.college, ''), COALESCE(comp_directories.project_source, ''), COALESCE(comp_directories.status, 0)`).
		Order("comp_directories.create_time DESC, comp_directories.id DESC").Rows()
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var stream *excelize.StreamWriter
	sheetNumber, sheetRow, count := 0, competitionExportSheetRows+1, 0
	for rows.Next() {
		if err := query.Statement.Context.Err(); err != nil {
			return 0, err
		}
		if sheetRow > competitionExportSheetRows {
			if stream != nil {
				if err := stream.Flush(); err != nil {
					return 0, err
				}
			}
			sheetNumber++
			stream, err = openCompetitionExportSheet(workbook, sheetNumber)
			if err != nil {
				return 0, err
			}
			sheetRow = 2
		}
		var code, name, level, organizer, undertaker, manager, college, source string
		var status int
		if err := rows.Scan(&code, &name, &level, &organizer, &undertaker, &manager, &college, &source, &status); err != nil {
			return 0, err
		}
		statusText := "未开始"
		switch status {
		case 1:
			statusText = "进行中"
		case 2:
			statusText = "已结束"
		}
		values := []any{code, name, level, organizer, undertaker, manager, college, source, statusText}
		for i, value := range values {
			if value == "" {
				values[i] = "-"
			}
		}
		if err := stream.SetRow(fmt.Sprintf("A%d", sheetRow), values); err != nil {
			return 0, err
		}
		sheetRow++
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if stream != nil {
		return count, stream.Flush()
	}
	return count, nil
}

func openCompetitionExportSheet(workbook *excelize.File, number int) (*excelize.StreamWriter, error) {
	name := competitionExportSheet
	if number == 1 {
		if err := workbook.SetSheetName(workbook.GetSheetName(0), name); err != nil {
			return nil, err
		}
	} else {
		name = fmt.Sprintf("%s%d", competitionExportSheet, number)
		if _, err := workbook.NewSheet(name); err != nil {
			return nil, err
		}
	}
	stream, err := workbook.NewStreamWriter(name)
	if err != nil {
		return nil, err
	}
	for i, width := range []float64{15, 30, 15, 25, 25, 20, 25, 25, 10} {
		if err := stream.SetColWidth(i+1, i+1, width); err != nil {
			return nil, err
		}
	}
	headers := []any{"赛事编号", "赛事名称", "赛事级别", "主办单位", "承办单位", "赛事负责人", "所属单位", "项目来源", "状态"}
	return stream, stream.SetRow("A1", headers)
}
