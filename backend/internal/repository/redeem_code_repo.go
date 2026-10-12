package repository

import (
	"context"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/redeemcode"
	"github.com/Wei-Shaw/sub2api/ent/redeemcodeclaim"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"

	entsql "entgo.io/ent/dialect/sql"
)

type redeemCodeRepository struct {
	client *dbent.Client
}

func NewRedeemCodeRepository(client *dbent.Client) service.RedeemCodeRepository {
	return &redeemCodeRepository{client: client}
}

func (r *redeemCodeRepository) Create(ctx context.Context, code *service.RedeemCode) error {
	created, err := clientFromContext(ctx, r.client).RedeemCode.Create().
		SetCode(code.Code).
		SetType(code.Type).
		SetValue(code.Value).
		SetStatus(code.Status).
		SetMaxUses(maxInt(code.MaxUses)).
		SetUsedCount(code.UsedCount).
		SetNotes(code.Notes).
		SetValidityDays(code.ValidityDays).
		SetNillableExpiresAt(code.ExpiresAt).
		SetNillableUsedBy(code.UsedBy).
		SetNillableUsedAt(code.UsedAt).
		SetNillableGroupID(code.GroupID).
		Save(ctx)
	if err == nil {
		code.ID = created.ID
		code.CreatedAt = created.CreatedAt
	}
	return err
}

func maxInt(value int) int {
	if value <= 0 {
		return 1
	}
	return value
}

func (r *redeemCodeRepository) CreateBatch(ctx context.Context, codes []service.RedeemCode) error {
	if len(codes) == 0 {
		return nil
	}

	builders := make([]*dbent.RedeemCodeCreate, 0, len(codes))
	for i := range codes {
		c := &codes[i]
		b := clientFromContext(ctx, r.client).RedeemCode.Create().
			SetCode(c.Code).
			SetType(c.Type).
			SetValue(c.Value).
			SetStatus(c.Status).
			SetMaxUses(maxInt(c.MaxUses)).
			SetUsedCount(c.UsedCount).
			SetNotes(c.Notes).
			SetValidityDays(c.ValidityDays).
			SetNillableExpiresAt(c.ExpiresAt).
			SetNillableUsedBy(c.UsedBy).
			SetNillableUsedAt(c.UsedAt).
			SetNillableGroupID(c.GroupID)
		builders = append(builders, b)
	}

	return clientFromContext(ctx, r.client).RedeemCode.CreateBulk(builders...).Exec(ctx)
}

func (r *redeemCodeRepository) GetByID(ctx context.Context, id int64) (*service.RedeemCode, error) {
	m, err := clientFromContext(ctx, r.client).RedeemCode.Query().
		Where(redeemcode.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrRedeemCodeNotFound
		}
		return nil, err
	}
	return redeemCodeEntityToService(m), nil
}

func (r *redeemCodeRepository) GetByCode(ctx context.Context, code string) (*service.RedeemCode, error) {
	m, err := r.client.RedeemCode.Query().
		Where(redeemcode.CodeEQ(code)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrRedeemCodeNotFound
		}
		return nil, err
	}
	return redeemCodeEntityToService(m), nil
}

func (r *redeemCodeRepository) Delete(ctx context.Context, id int64) error {
	exists, err := clientFromContext(ctx, r.client).RedeemCodeClaim.Query().Where(redeemcodeclaim.RedeemCodeIDEQ(id)).Exist(ctx)
	if err != nil {
		return err
	}
	if exists {
		return service.ErrRedeemCodeHasClaims
	}
	_, err = clientFromContext(ctx, r.client).RedeemCode.Delete().Where(redeemcode.IDEQ(id)).Exec(ctx)
	return err
}

func (r *redeemCodeRepository) List(ctx context.Context, params pagination.PaginationParams) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", "")
}

func (r *redeemCodeRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, codeType, status, search string) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	q := r.client.RedeemCode.Query()

	if codeType != "" {
		q = q.Where(redeemcode.TypeEQ(codeType))
	}
	if status != "" {
		now := time.Now()
		switch status {
		case service.StatusExpired:
			q = q.Where(redeemcode.Or(
				redeemcode.StatusEQ(service.StatusExpired),
				redeemcode.And(
					redeemcode.StatusEQ(service.StatusUnused),
					redeemcode.ExpiresAtNotNil(),
					redeemcode.ExpiresAtLTE(now),
				),
			))
		case service.StatusUnused:
			q = q.Where(
				redeemcode.StatusEQ(service.StatusUnused),
				redeemcode.Or(
					redeemcode.ExpiresAtIsNil(),
					redeemcode.ExpiresAtGT(now),
				),
			)
		default:
			q = q.Where(redeemcode.StatusEQ(status))
		}
	}
	if search != "" {
		q = q.Where(
			redeemcode.Or(
				redeemcode.CodeContainsFold(search),
				redeemcode.HasUserWith(user.EmailContainsFold(search)),
			),
		)
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	codesQuery := q.
		WithUser().
		WithGroup().
		Offset(params.Offset()).
		Limit(params.Limit())
	for _, order := range redeemCodeListOrder(params) {
		codesQuery = codesQuery.Order(order)
	}

	codes, err := codesQuery.All(ctx)
	if err != nil {
		return nil, nil, err
	}

	outCodes := redeemCodeEntitiesToService(codes)

	return outCodes, paginationResultFromTotal(int64(total), params), nil
}

func redeemCodeListOrder(params pagination.PaginationParams) []func(*entsql.Selector) {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := params.NormalizedSortOrder(pagination.SortOrderDesc)

	var field string
	switch sortBy {
	case "type":
		field = redeemcode.FieldType
	case "value":
		field = redeemcode.FieldValue
	case "status":
		field = redeemcode.FieldStatus
	case "used_at":
		field = redeemcode.FieldUsedAt
	case "created_at":
		field = redeemcode.FieldCreatedAt
	case "expires_at":
		field = redeemcode.FieldExpiresAt
	case "code":
		field = redeemcode.FieldCode
	default:
		field = redeemcode.FieldID
	}

	if sortOrder == pagination.SortOrderAsc {
		return []func(*entsql.Selector){dbent.Asc(field), dbent.Asc(redeemcode.FieldID)}
	}
	return []func(*entsql.Selector){dbent.Desc(field), dbent.Desc(redeemcode.FieldID)}
}

func (r *redeemCodeRepository) Update(ctx context.Context, code *service.RedeemCode) error {
	up := r.client.RedeemCode.UpdateOneID(code.ID).
		SetCode(code.Code).
		SetType(code.Type).
		SetValue(code.Value).
		SetStatus(code.Status).
		SetNotes(code.Notes).
		SetValidityDays(code.ValidityDays)

	if code.UsedBy != nil {
		up.SetUsedBy(*code.UsedBy)
	} else {
		up.ClearUsedBy()
	}
	if code.UsedAt != nil {
		up.SetUsedAt(*code.UsedAt)
	} else {
		up.ClearUsedAt()
	}
	if code.GroupID != nil {
		up.SetGroupID(*code.GroupID)
	} else {
		up.ClearGroupID()
	}
	if code.ExpiresAt != nil {
		up.SetExpiresAt(*code.ExpiresAt)
	} else {
		up.ClearExpiresAt()
	}

	updated, err := up.Save(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return service.ErrRedeemCodeNotFound
		}
		return err
	}
	code.CreatedAt = updated.CreatedAt
	return nil
}

func (r *redeemCodeRepository) BatchUpdate(ctx context.Context, ids []int64, fields service.RedeemCodeBatchUpdateFields) (int64, error) {
	uniqueIDs := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) == 0 {
		return 0, nil
	}

	if tx := dbent.TxFromContext(ctx); tx != nil {
		return r.batchUpdate(ctx, tx.Client(), uniqueIDs, fields)
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return 0, err
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	defer func() { _ = tx.Rollback() }()

	updated, err := r.batchUpdate(txCtx, tx.Client(), uniqueIDs, fields)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return updated, nil
}

func (r *redeemCodeRepository) batchUpdate(ctx context.Context, client *dbent.Client, ids []int64, fields service.RedeemCodeBatchUpdateFields) (int64, error) {
	existing, err := client.RedeemCode.Query().
		Where(redeemcode.IDIn(ids...)).
		All(ctx)
	if err != nil {
		return 0, err
	}
	if len(existing) != len(ids) {
		return 0, service.ErrRedeemCodeNotFound
	}
	if fields.TouchesUsedSensitiveFields() {
		for _, code := range existing {
			if code.Status == service.StatusUsed {
				return 0, service.ErrRedeemCodeUsed
			}
		}
	}

	up := client.RedeemCode.Update().Where(redeemcode.IDIn(ids...))
	if fields.Status != nil {
		up.SetStatus(*fields.Status)
	}
	if fields.Notes != nil {
		up.SetNotes(*fields.Notes)
	}
	if fields.ExpiresAt.Set {
		if fields.ExpiresAt.Value != nil {
			up.SetExpiresAt(*fields.ExpiresAt.Value)
		} else {
			up.ClearExpiresAt()
		}
	}
	if fields.GroupID.Set {
		if fields.GroupID.Value != nil {
			up.SetGroupID(*fields.GroupID.Value)
		} else {
			up.ClearGroupID()
		}
	}

	affected, err := up.Save(ctx)
	if err != nil {
		return 0, err
	}
	if affected != len(ids) {
		return 0, service.ErrRedeemCodeNotFound
	}
	return int64(affected), nil
}

func (r *redeemCodeRepository) Use(ctx context.Context, id, userID int64) error {
	now := time.Now()
	client := clientFromContext(ctx, r.client)
	affected, err := client.RedeemCode.Update().
		Where(redeemcode.IDEQ(id), redeemcode.StatusEQ(service.StatusUnused)).
		SetStatus(service.StatusUsed).
		SetUsedCount(1).
		SetUsedBy(userID).
		SetUsedAt(now).
		Save(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrRedeemCodeUsed
	}
	return nil
}

func (r *redeemCodeRepository) Claim(ctx context.Context, codeID, userID int64, email string) error {
	if dbent.TxFromContext(ctx) == nil {
		tx, err := r.client.Tx(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		if err = r.Claim(dbent.NewTxContext(ctx, tx), codeID, userID, email); err != nil {
			return err
		}
		return tx.Commit()
	}
	client := clientFromContext(ctx, r.client)
	code, err := client.RedeemCode.Query().Where(redeemcode.IDEQ(codeID)).ForUpdate().Only(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrRedeemCodeNotFound, nil)
	}
	if code.Status != service.StatusUnused || code.UsedCount >= code.MaxUses {
		return service.ErrRedeemCodeUsed
	}
	if code.ExpiresAt != nil && !code.ExpiresAt.After(time.Now()) {
		return service.ErrRedeemCodeExpired
	}
	owner, err := client.User.Query().Where(user.IDEQ(userID), user.DeletedAtIsNil()).ForUpdate().Only(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUserNotFound, nil)
	}
	email = strings.ToLower(strings.TrimSpace(owner.Email))
	if email == "" {
		return service.ErrRedeemEmailRequired
	}
	exists, err := client.RedeemCodeClaim.Query().Where(redeemcodeclaim.RedeemCodeIDEQ(codeID), redeemcodeclaim.Or(redeemcodeclaim.UserIDEQ(userID), redeemcodeclaim.EmailEQ(email))).Exist(ctx)
	if err != nil {
		return err
	}
	if exists {
		return service.ErrRedeemCodeAlreadyClaimed
	}
	_, err = client.RedeemCodeClaim.Create().SetRedeemCodeID(codeID).SetUserID(userID).SetEmail(email).
		SetType(code.Type).SetValue(code.Value).SetNillableGroupID(code.GroupID).SetValidityDays(code.ValidityDays).Save(ctx)
	if err != nil {
		return translatePersistenceError(err, nil, service.ErrRedeemCodeAlreadyClaimed)
	}
	update := client.RedeemCode.UpdateOneID(codeID).SetUsedCount(code.UsedCount + 1)
	if code.UsedCount+1 >= code.MaxUses {
		update.SetStatus(service.StatusUsed)
	}
	if code.UsedBy == nil {
		update.SetUsedBy(userID)
	}
	if code.UsedAt == nil {
		update.SetUsedAt(time.Now())
	}
	_, err = update.Save(ctx)
	return err
}

const redeemUserHistorySQL = `WITH history AS (
 SELECT rc.id, rc.code, rc.type, rc.value, rc.status, rc.used_by, rc.used_at, COALESCE(rc.notes,'') AS notes, rc.created_at, rc.expires_at, rc.group_id, rc.validity_days, rc.max_uses, rc.used_count
 FROM redeem_codes rc WHERE rc.used_by=$1 AND NOT EXISTS (SELECT 1 FROM redeem_code_claims claims WHERE claims.redeem_code_id=rc.id)
 UNION ALL
 SELECT rc.id, rc.code, claims.type, claims.value, 'used', claims.user_id, claims.claimed_at, COALESCE(rc.notes,''), rc.created_at, rc.expires_at, claims.group_id, claims.validity_days, rc.max_uses, rc.used_count
 FROM redeem_code_claims claims JOIN redeem_codes rc ON rc.id=claims.redeem_code_id WHERE claims.user_id=$1
)
`

func (r *redeemCodeRepository) ListByUser(ctx context.Context, userID int64, limit int) ([]service.RedeemCode, error) {
	if limit <= 0 {
		limit = 10
	}
	codes, _, err := r.ListByUserPaginated(ctx, userID, pagination.PaginationParams{Page: 1, PageSize: limit}, "")
	return codes, err
}

func (r *redeemCodeRepository) ListByUserPaginated(ctx context.Context, userID int64, params pagination.PaginationParams, codeType string) ([]service.RedeemCode, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	countRows, err := client.QueryContext(ctx, redeemUserHistorySQL+"SELECT COUNT(*) FROM history WHERE ($2='' OR type=$2)", userID, codeType)
	if err != nil {
		return nil, nil, err
	}
	var total int64
	if !countRows.Next() {
		_ = countRows.Close()
		return nil, nil, service.ErrRedeemCodeNotFound
	}
	if err = countRows.Scan(&total); err != nil {
		_ = countRows.Close()
		return nil, nil, err
	}
	_ = countRows.Close()
	rows, err := client.QueryContext(ctx, redeemUserHistorySQL+"SELECT id,code,type,value,status,used_by,used_at,notes,created_at,expires_at,group_id,validity_days,max_uses,used_count FROM history WHERE ($2='' OR type=$2) ORDER BY used_at DESC NULLS LAST,id DESC OFFSET $3 LIMIT $4", userID, codeType, params.Offset(), params.Limit())
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()
	codes := make([]service.RedeemCode, 0, params.Limit())
	for rows.Next() {
		var code service.RedeemCode
		if err = rows.Scan(&code.ID, &code.Code, &code.Type, &code.Value, &code.Status, &code.UsedBy, &code.UsedAt, &code.Notes, &code.CreatedAt, &code.ExpiresAt, &code.GroupID, &code.ValidityDays, &code.MaxUses, &code.UsedCount); err != nil {
			return nil, nil, err
		}
		codes = append(codes, code)
	}
	if err = rows.Err(); err != nil {
		return nil, nil, err
	}
	_ = rows.Close()
	groups := make(map[int64]*service.Group)
	for index := range codes {
		if codes[index].GroupID == nil {
			continue
		}
		groupID := *codes[index].GroupID
		group, loaded := groups[groupID]
		if !loaded {
			entity, lookupErr := client.Group.Get(ctx, groupID)
			if lookupErr != nil && !dbent.IsNotFound(lookupErr) {
				return nil, nil, lookupErr
			}
			if entity != nil {
				group = groupEntityToService(entity)
			}
			groups[groupID] = group
		}
		codes[index].Group = group
	}
	return codes, paginationResultFromTotal(total, params), nil
}

func (r *redeemCodeRepository) SumPositiveBalanceByUser(ctx context.Context, userID int64) (float64, error) {
	rows, err := clientFromContext(ctx, r.client).QueryContext(ctx, redeemUserHistorySQL+"SELECT COALESCE(SUM(value),0) FROM history WHERE value > 0 AND type IN ('balance','admin_balance')", userID)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	var sum float64
	if rows.Next() {
		err = rows.Scan(&sum)
	}
	if err != nil {
		return 0, err
	}
	return sum, rows.Err()
}

func redeemCodeEntityToService(m *dbent.RedeemCode) *service.RedeemCode {
	if m == nil {
		return nil
	}
	out := &service.RedeemCode{
		ID:           m.ID,
		Code:         m.Code,
		Type:         m.Type,
		Value:        m.Value,
		Status:       m.Status,
		MaxUses:      m.MaxUses,
		UsedCount:    m.UsedCount,
		UsedBy:       m.UsedBy,
		UsedAt:       m.UsedAt,
		Notes:        derefString(m.Notes),
		CreatedAt:    m.CreatedAt,
		ExpiresAt:    m.ExpiresAt,
		GroupID:      m.GroupID,
		ValidityDays: m.ValidityDays,
	}
	if m.Edges.User != nil {
		out.User = userEntityToService(m.Edges.User)
	}
	if m.Edges.Group != nil {
		out.Group = groupEntityToService(m.Edges.Group)
	}
	return out
}

func redeemCodeEntitiesToService(models []*dbent.RedeemCode) []service.RedeemCode {
	out := make([]service.RedeemCode, 0, len(models))
	for i := range models {
		if s := redeemCodeEntityToService(models[i]); s != nil {
			out = append(out, *s)
		}
	}
	return out
}
