package sqlite

import (
	"context"
	"polaris-hermes/internal/domain"
)

type RouteRepo struct{}

func NewRouteRepo() *RouteRepo {
	return &RouteRepo{}
}

// GetUserCustomRoutes 获取用户配置的所有强制自定义路由 (1对1)
func (r *RouteRepo) GetUserCustomRoutes(ctx context.Context) ([]domain.UserCustomRoute, error) {
	query := `
		SELECT id, requested_model_id, target_user_model_id, is_active
		FROM user_custom_routes
		WHERE is_active = 1
	`
	rows, err := DB().QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var routes []domain.UserCustomRoute
	for rows.Next() {
		var rt domain.UserCustomRoute
		err := rows.Scan(
			&rt.ID, &rt.RequestedModelID, &rt.TargetUserModelID, &rt.IsActive,
		)
		if err != nil {
			return nil, err
		}
		routes = append(routes, rt)
	}
	return routes, nil
}

// GetAllUserCustomRoutes 获取所有用户配置的路由 (含禁用)
func (r *RouteRepo) GetAllUserCustomRoutes(ctx context.Context) ([]domain.UserCustomRoute, error) {
	query := `
		SELECT id, requested_model_id, target_user_model_id, is_active
		FROM user_custom_routes
	`
	rows, err := DB().QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var routes []domain.UserCustomRoute
	for rows.Next() {
		var rt domain.UserCustomRoute
		err := rows.Scan(
			&rt.ID, &rt.RequestedModelID, &rt.TargetUserModelID, &rt.IsActive,
		)
		if err != nil {
			return nil, err
		}
		routes = append(routes, rt)
	}
	return routes, nil
}

func (r *RouteRepo) CreateUserCustomRoute(ctx context.Context, rt *domain.UserCustomRoute) error {
	query := `
		INSERT INTO user_custom_routes (requested_model_id, target_user_model_id, is_active)
		VALUES (?, ?, ?)
	`
	res, err := DB().ExecContext(ctx, query, rt.RequestedModelID, rt.TargetUserModelID, rt.IsActive)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err == nil {
		rt.ID = int(id)
	}
	return nil
}

func (r *RouteRepo) UpdateUserCustomRoute(ctx context.Context, rt *domain.UserCustomRoute) error {
	query := `
		UPDATE user_custom_routes
		SET requested_model_id = ?, target_user_model_id = ?, is_active = ?
		WHERE id = ?
	`
	_, err := DB().ExecContext(ctx, query, rt.RequestedModelID, rt.TargetUserModelID, rt.IsActive, rt.ID)
	return err
}

func (r *RouteRepo) DeleteUserCustomRoute(ctx context.Context, id int) error {
	query := `DELETE FROM user_custom_routes WHERE id = ?`
	_, err := DB().ExecContext(ctx, query, id)
	return err
}
