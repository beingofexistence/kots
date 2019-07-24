package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/replicatedhq/ship-cluster/worker/pkg/types"
)

func (s *SQLStore) CreateWatchVersion(ctx context.Context, watchID string, versionLabel string, status string, sourceBranch string, sequence int, pullRequestNumber int, commitSHA string, setCurrent bool, parentSequence *int) error {
	if sourceBranch != "" && pullRequestNumber > 0 {
		query := `insert into watch_version (watch_id, created_at, version_label, status, source_branch, sequence, pullrequest_number, commit_sha, parent_sequence)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

		_, err := s.db.ExecContext(ctx, query, watchID, time.Now(), versionLabel, status, sourceBranch, sequence, pullRequestNumber, commitSHA, parentSequence)
		if err != nil {
			return errors.Wrap(err, "create gitops watch version")
		}
	} else {
		query := `insert into watch_version (watch_id, created_at, version_label, status, sequence, parent_sequence) values
		($1, $2, $3, $4, $5, $6)`

		_, err := s.db.ExecContext(ctx, query, watchID, time.Now(), versionLabel, status, sequence, parentSequence)
		if err != nil {
			return errors.Wrap(err, "create ship watch version")
		}
	}

	if setCurrent {
		query := `update watch set current_sequence = $1 where id = $2`
		_, err := s.db.ExecContext(ctx, query, sequence, watchID)
		if err != nil {
			return errors.Wrap(err, "set current sequence")
		}
	}
	return nil
}

func (s *SQLStore) GetMostRecentWatchVersion(ctx context.Context, watchID string) (*types.WatchVersion, error) {
	query := `select watch_id, created_at, version_label, status, source_branch, sequence, pullrequest_number
	from watch_version where watch_id = $1 order by sequence desc limit 1`

	row := s.db.QueryRowContext(ctx, query, watchID)

	watchVersion := types.WatchVersion{}
	var sourceBranch sql.NullString
	var pullRequestNumber sql.NullInt64

	if err := row.Scan(&watchVersion.WatchID, &watchVersion.CreatedAt, &watchVersion.VersionLabel, &watchVersion.Status,
		&sourceBranch, &watchVersion.Sequence, &pullRequestNumber); err != nil {
		return nil, errors.Wrap(err, "read watch version")
	}

	if sourceBranch.Valid {
		watchVersion.SourceBranch = sourceBranch.String
	}
	if pullRequestNumber.Valid {
		watchVersion.PullRequestNumber = int(pullRequestNumber.Int64)
	}

	return &watchVersion, nil
}

func (s *SQLStore) GetOneWatchVersion(ctx context.Context, watchID string, sequence int) (*types.WatchVersion, error) {
	query := `select watch_id, created_at, version_label, status, source_branch, sequence, pullrequest_number
	from watch_version where watch_id = $1 and sequence = $2`

	row := s.db.QueryRowContext(ctx, query, watchID, sequence)

	watchVersion := types.WatchVersion{}
	var sourceBranch sql.NullString
	var pullRequestNumber sql.NullInt64

	if err := row.Scan(&watchVersion.WatchID, &watchVersion.CreatedAt, &watchVersion.VersionLabel, &watchVersion.Status,
		&sourceBranch, &watchVersion.Sequence, &pullRequestNumber); err != nil {
		return nil, errors.Wrap(err, "read watch version")
	}

	if sourceBranch.Valid {
		watchVersion.SourceBranch = sourceBranch.String
	}
	if pullRequestNumber.Valid {
		watchVersion.PullRequestNumber = int(pullRequestNumber.Int64)
	}

	return &watchVersion, nil
}
