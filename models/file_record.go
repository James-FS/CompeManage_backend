package models

import "time"

type FileRecord struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	FileHash     string    `gorm:"type:varchar(64);uniqueIndex:idx_file_records_hash_biz_type,priority:1;not null;comment:文件MD5哈希值" json:"file_hash"`
	OriginalName string    `gorm:"type:varchar(255);not null;comment:原始文件名" json:"original_name"`
	StoragePath  string    `gorm:"type:varchar(512);not null;comment:存储路径" json:"storage_path"`
	FileSize     int64     `gorm:"type:bigint;not null;comment:文件大小(字节)" json:"file_size"`
	FileExt      string    `gorm:"type:varchar(16);not null;comment:文件扩展名" json:"file_ext"`
	BizType      string    `gorm:"type:varchar(32);index;uniqueIndex:idx_file_records_hash_biz_type,priority:2;comment:业务类型(notice/reg_attachment/competition_work/award_proof/award_cert/declare_attachment/summary_attachment/temp)" json:"biz_type"`
	UploaderID   uint      `gorm:"index;comment:上传人用户ID" json:"uploader_id"`
	CreateTime   time.Time `gorm:"column:create_time;autoCreateTime" json:"create_time"`
}
