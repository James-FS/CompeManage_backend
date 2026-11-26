package database

import "gorm.io/gorm"

// NULL marks older rows. A declared college alone does not identify the
// recording account's unit. An explicitly saved empty unit stays empty.
func BackfillCompetitionProjectSources(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`UPDATE comp_declarations d
			LEFT JOIN users u ON u.id = d.created_by
			LEFT JOIN colleges account_college ON account_college.name = TRIM(u.college)
				AND account_college.is_valid = TRUE
			SET d.project_source = TRIM(u.college),
			    d.project_source_college_id = account_college.id
			WHERE d.project_source IS NULL AND TRIM(u.college) <> ''`).Error; err != nil {
			return err
		}
		// For old declarations entered into the directory, created_by identified the auditor.
		return tx.Exec(`UPDATE comp_directories cd
			LEFT JOIN comp_declarations d ON d.id = cd.declare_id
			LEFT JOIN users u ON u.id = cd.created_by
			LEFT JOIN colleges account_college ON account_college.name = TRIM(u.college)
				AND account_college.is_valid = TRUE
			SET cd.project_source = CASE WHEN cd.source = 2 THEN d.project_source ELSE TRIM(u.college) END,
			    cd.project_source_college_id = CASE WHEN cd.source = 2
			        THEN d.project_source_college_id ELSE account_college.id END
			WHERE cd.project_source IS NULL AND (
			    (cd.source = 2 AND d.project_source IS NOT NULL)
			    OR (cd.source = 1 AND TRIM(u.college) <> '')
			)`).Error
	})
}
