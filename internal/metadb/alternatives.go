package metadb

import "fmt"

// TorrentAlternative stores a ranked torrent candidate for a content item.
// Rank 1 is the currently active torrent. Ranks 2-20 are potential alternatives.
type TorrentAlternative struct {
	ContentID        string `json:"content_id"`
	ContentType      string `json:"content_type"` // "tv" or "movie"
	Rank             int    `json:"rank"`
	Hash             string `json:"hash"`
	Title            string `json:"title"`
	Size             int64  `json:"size_bytes"`
	Seeders          int    `json:"seeders"`
	QualityScore     int    `json:"quality_score"`
	Status           string `json:"status"` // active, tested_no_better, verified_healthy, verified_slow, dead
	LastHealthCheck  int64  `json:"last_health_check"`
	AvgSpeedKBps     int64  `json:"avg_speed_kbps"`
	ReplacementCount int    `json:"replacement_count"`
}

// UpsertAlternative inserts or replaces a single alternative entry.
func (d *DB) UpsertAlternative(a TorrentAlternative) error {
	_, err := d.db.Exec(
		`INSERT OR REPLACE INTO torrent_alternatives
		 (content_id, content_type, rank, hash, title, size_bytes, seeders, quality_score, status, last_health_check, avg_speed_kbps, replacement_count)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ContentID, a.ContentType, a.Rank, a.Hash, a.Title, a.Size, a.Seeders,
		a.QualityScore, a.Status, a.LastHealthCheck, a.AvgSpeedKBps, a.ReplacementCount,
	)
	return err
}

// GetAlternative retrieves a single alternative by content_id and hash.
func (d *DB) GetAlternative(contentID, hash string) (*TorrentAlternative, bool, error) {
	var a TorrentAlternative
	err := d.db.QueryRow(
		`SELECT content_id, content_type, rank, hash, title, size_bytes, seeders, quality_score,
		        status, last_health_check, avg_speed_kbps, replacement_count
		 FROM torrent_alternatives WHERE content_id = ? AND hash = ?`,
		contentID, hash,
	).Scan(&a.ContentID, &a.ContentType, &a.Rank, &a.Hash, &a.Title, &a.Size,
		&a.Seeders, &a.QualityScore, &a.Status, &a.LastHealthCheck, &a.AvgSpeedKBps, &a.ReplacementCount)
	if err != nil {
		return nil, false, nil
	}
	return &a, true, nil
}

// GetAlternativesByContent returns all alternatives for a content ID, ordered by rank.
func (d *DB) GetAlternativesByContent(contentID string) ([]TorrentAlternative, error) {
	rows, err := d.db.Query(
		`SELECT content_id, content_type, rank, hash, title, size_bytes, seeders, quality_score,
		        status, last_health_check, avg_speed_kbps, replacement_count
		 FROM torrent_alternatives WHERE content_id = ? ORDER BY rank`,
		contentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []TorrentAlternative
	for rows.Next() {
		var a TorrentAlternative
		if err := rows.Scan(&a.ContentID, &a.ContentType, &a.Rank, &a.Hash, &a.Title, &a.Size,
			&a.Seeders, &a.QualityScore, &a.Status, &a.LastHealthCheck, &a.AvgSpeedKBps, &a.ReplacementCount); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// GetNextBestAlternative returns the best (lowest rank) alternative that hasn't been
// tested yet (status = 'active') and is not the current hash.
func (d *DB) GetNextBestAlternative(contentID, currentHash string) (*TorrentAlternative, bool, error) {
	var a TorrentAlternative
	err := d.db.QueryRow(
		`SELECT content_id, content_type, rank, hash, title, size_bytes, seeders, quality_score,
		        status, last_health_check, avg_speed_kbps, replacement_count
		 FROM torrent_alternatives
		 WHERE content_id = ? AND hash != ? AND status = 'active'
		 ORDER BY rank LIMIT 1`,
		contentID, currentHash,
	).Scan(&a.ContentID, &a.ContentType, &a.Rank, &a.Hash, &a.Title, &a.Size,
		&a.Seeders, &a.QualityScore, &a.Status, &a.LastHealthCheck, &a.AvgSpeedKBps, &a.ReplacementCount)
	if err != nil {
		return nil, false, nil
	}
	return &a, true, nil
}

// SaveAlternativesForContent replaces all alternatives for a content ID with the given list.
// Used during sync to save the top 20 filtered results.
func (d *DB) SaveAlternativesForContent(contentID string, alts []TorrentAlternative) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM torrent_alternatives WHERE content_id = ?`, contentID); err != nil {
		return err
	}

	stmt, err := tx.Prepare(
		`INSERT INTO torrent_alternatives
		 (content_id, content_type, rank, hash, title, size_bytes, seeders, quality_score, status, last_health_check, avg_speed_kbps, replacement_count)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, a := range alts {
		if _, err := stmt.Exec(a.ContentID, a.ContentType, a.Rank, a.Hash, a.Title, a.Size,
			a.Seeders, a.QualityScore, a.Status, a.LastHealthCheck, a.AvgSpeedKBps, a.ReplacementCount); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteAlternativesByContent removes all alternatives for a content ID.
func (d *DB) DeleteAlternativesByContent(contentID string) error {
	_, err := d.db.Exec(`DELETE FROM torrent_alternatives WHERE content_id = ?`, contentID)
	return err
}

// UpdateAlternativeStatus updates the status of a single alternative.
func (d *DB) UpdateAlternativeStatus(contentID, hash, status string) error {
	_, err := d.db.Exec(
		`UPDATE torrent_alternatives SET status = ? WHERE content_id = ? AND hash = ?`,
		status, contentID, hash,
	)
	return err
}

// UpdateAlternativeHealth updates the health metrics of an alternative.
func (d *DB) UpdateAlternativeHealth(contentID, hash string, status string, speedKBps int64, seeders int, nowUnix int64) error {
	_, err := d.db.Exec(
		`UPDATE torrent_alternatives
		 SET status = ?, avg_speed_kbps = ?, seeders = ?, last_health_check = ?
		 WHERE content_id = ? AND hash = ?`,
		status, speedKBps, seeders, nowUnix, contentID, hash,
	)
	return err
}

// GetUnhealthyAlternatives returns all alternatives with status 'dead' or 'verified_slow',
// ordered by last_health_check ascending (oldest first).
func (d *DB) GetUnhealthyAlternatives() ([]TorrentAlternative, error) {
	rows, err := d.db.Query(
		`SELECT content_id, content_type, rank, hash, title, size_bytes, seeders, quality_score,
		        status, last_health_check, avg_speed_kbps, replacement_count
		 FROM torrent_alternatives
		 WHERE status IN ('dead', 'verified_slow')
		 ORDER BY last_health_check ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []TorrentAlternative
	for rows.Next() {
		var a TorrentAlternative
		if err := rows.Scan(&a.ContentID, &a.ContentType, &a.Rank, &a.Hash, &a.Title, &a.Size,
			&a.Seeders, &a.QualityScore, &a.Status, &a.LastHealthCheck, &a.AvgSpeedKBps, &a.ReplacementCount); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// IncrementReplacementCount increments the replacement counter for an alternative.
func (d *DB) IncrementReplacementCount(contentID, hash string) error {
	_, err := d.db.Exec(
		`UPDATE torrent_alternatives SET replacement_count = replacement_count + 1
		 WHERE content_id = ? AND hash = ?`,
		contentID, hash,
	)
	return err
}

// CountAlternativesByStatus returns the count of alternatives per status.
func (d *DB) CountAlternativesByStatus() (map[string]int, error) {
	rows, err := d.db.Query(
		`SELECT status, COUNT(*) FROM torrent_alternatives GROUP BY status`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		result[status] = count
	}
	return result, rows.Err()
}

// FormatAlternativeStatus returns a human-readable status string.
func FormatAlternativeStatus(status string) string {
	switch status {
	case "active":
		return "Active"
	case "tested_no_better":
		return "Tested (No Better)"
	case "verified_healthy":
		return "Verified Healthy"
	case "verified_slow":
		return "Verified Slow"
	case "dead":
		return "Dead"
	default:
		return fmt.Sprintf("Unknown (%s)", status)
	}
}
