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
