package pos

import (
	"fmt"
	"strconv"
	"time"
)

const validatorCols = `id, uid, address, name, total_stake, self_stake,
	delegated_stake, commission_rate, status, joined_at, jailed_at,
	uptime, blocks_proposed, blocks_missed`

func scanValidator(scanner interface{ Scan(...any) error }) (*Validator, error) {
	var v Validator
	err := scanner.Scan(
		&v.ID, &v.UID, &v.Address, &v.Name, &v.TotalStake, &v.SelfStake,
		&v.DelegatedStake, &v.CommissionRate, &v.Status, &v.JoinedAt, &v.JailedAt,
		&v.Uptime, &v.BlocksProposed, &v.BlocksMissed)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// RegisterValidator 註冊成為驗證人。
func (s *Store) RegisterValidator(uid int64, address, name string, commission float64) (*Validator, error) {
	if commission < 0 || commission > 0.2 {
		return nil, fmt.Errorf("傭金率必須在 0-20%% 之間")
	}
	now := time.Now().Unix()

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var existing int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM validators WHERE address=?`, address).
		Scan(&existing); err != nil {
		return nil, err
	}
	if existing > 0 {
		return nil, fmt.Errorf("該地址已註冊為驗證人")
	}

	res, err := tx.Exec(
		`INSERT INTO validators (uid,address,name,commission_rate,status,joined_at)
		 VALUES (?,?,?,?,'active',?)`,
		uid, address, name, commission, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()

	var tv int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM validators`).Scan(&tv); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(
		`INSERT OR REPLACE INTO pos_params (key,value,updated_at) VALUES (?,?,?)`,
		"total_validators", strconv.Itoa(tv), now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetValidatorInfo(id)
}

// GetValidatorInfo 返回驗證人詳情。
func (s *Store) GetValidatorInfo(id int64) (*Validator, error) {
	return scanValidator(s.db.QueryRow(
		`SELECT `+validatorCols+` FROM validators WHERE id=?`, id))
}

// GetValidators 返回活躍驗證人（按總質押降序）。
func (s *Store) GetValidators(limit int) ([]Validator, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT `+validatorCols+` FROM validators WHERE status='active'
		 ORDER BY total_stake DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Validator
	for rows.Next() {
		v, err := scanValidator(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

// GetTopValidators 返回前 N 名驗證人（當前驗證人集合）。
func (s *Store) GetTopValidators(count int) ([]Validator, error) {
	if count <= 0 {
		count = s.cfg.ValidatorCount
	}
	return s.GetValidators(count)
}
