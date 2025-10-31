package kb

import (
	"github.com/gin-gonic/gin"
	"github.com/yaoapp/gou/model"
	"github.com/yaoapp/yao/openapi/oauth/authorized"
	"github.com/yaoapp/yao/openapi/oauth/types"
)

// AuthFilter applies permission-based filtering to query wheres
// This function builds where clauses based on the user's authorization constraints
// It supports TeamOnly and OwnerOnly constraints for data access control
//
// Parameters:
//   - c: gin.Context containing authorization information
//   - authInfo: authorized information extracted from the context
//
// Returns:
//   - []model.QueryWhere: array of where clauses to apply to the query
func AuthFilter(c *gin.Context, authInfo *types.AuthorizedInfo) []model.QueryWhere {
	if authInfo == nil {
		return []model.QueryWhere{}
	}

	var wheres []model.QueryWhere
	scope := authInfo.AccessScope()

	// Team only - User can access:
	// 1. Public records (public = true)
	// 2. Records in their team where:
	//    - They created the record (__yao_created_by matches)
	//    - OR the record is shared with team (share = "team")
	if authInfo.Constraints.TeamOnly && authorized.IsTeamMember(c) {
		wheres = append(wheres, model.QueryWhere{
			Wheres: []model.QueryWhere{
				{Column: "public", Value: true, Method: "orwhere"},
				{Wheres: []model.QueryWhere{
					{Column: "__yao_team_id", Value: scope.TeamID},
					{Wheres: []model.QueryWhere{
						{Column: "__yao_created_by", Value: scope.CreatedBy},
						{Column: "share", Value: "team", Method: "orwhere"},
					}},
				}, Method: "orwhere"},
			},
		})
		return wheres
	}

	// Owner only - User can access:
	// 1. Public records (public = true)
	// 2. Records they created where:
	//    - __yao_team_id is null (not team records)
	//    - __yao_created_by matches their user ID
	if authInfo.Constraints.OwnerOnly && authInfo.UserID != "" {
		wheres = append(wheres, model.QueryWhere{
			Wheres: []model.QueryWhere{
				{Column: "public", Value: true, Method: "orwhere"},
				{Wheres: []model.QueryWhere{
					{Column: "__yao_team_id", OP: "null"},
					{Column: "__yao_created_by", Value: scope.CreatedBy},
				}, Method: "orwhere"},
			},
		})
		return wheres
	}

	return wheres
}
