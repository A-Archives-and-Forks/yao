package user

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yaoapp/gou/model"
	"github.com/yaoapp/gou/process"
	"github.com/yaoapp/kun/exception"
	"github.com/yaoapp/kun/log"
	"github.com/yaoapp/kun/maps"
	"github.com/yaoapp/yao/openapi/oauth"
	"github.com/yaoapp/yao/openapi/oauth/authorized"
	"github.com/yaoapp/yao/openapi/response"
)

// Member Management Handlers

// GinMemberList handles GET /teams/:team_id/members - Get team members with advanced filtering
func GinMemberList(c *gin.Context) {
	authInfo := authorized.GetInfo(c)
	if authInfo == nil || authInfo.UserID == "" {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidClient.Code,
			ErrorDescription: "User not authenticated",
		}
		response.RespondWithError(c, response.StatusUnauthorized, errorResp)
		return
	}

	teamID := c.Param("id")
	if teamID == "" {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidRequest.Code,
			ErrorDescription: "Team ID is required",
		}
		response.RespondWithError(c, response.StatusBadRequest, errorResp)
		return
	}

	// Parse request parameters
	var req MemberListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		// Provide a more user-friendly error message
		errMsg := "Invalid query parameters"
		if strings.Contains(err.Error(), "parsing") {
			errMsg = "Invalid query parameter format. Please check page, pagesize, and other numeric values."
		}
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidRequest.Code,
			ErrorDescription: errMsg,
		}
		response.RespondWithError(c, response.StatusBadRequest, errorResp)
		return
	}

	// Set default values
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	if req.PageSize > 100 {
		req.PageSize = 100
	}
	if req.Order == "" {
		req.Order = "created_at desc"
	}

	// Parse fields from comma-separated string if provided
	if fieldsStr := c.Query("fields"); fieldsStr != "" {
		req.Fields = strings.Split(fieldsStr, ",")
		// Trim spaces from field names
		for i, field := range req.Fields {
			req.Fields[i] = strings.TrimSpace(field)
		}
	}

	// Call business logic
	result, err := memberList(c.Request.Context(), authInfo.UserID, teamID, &req)
	if err != nil {
		log.Error("Failed to get team members: %v", err)
		// Check error type for appropriate response
		if strings.Contains(err.Error(), "not found") {
			errorResp := &response.ErrorResponse{
				Code:             response.ErrInvalidRequest.Code,
				ErrorDescription: "Team not found",
			}
			response.RespondWithError(c, response.StatusNotFound, errorResp)
		} else if strings.Contains(err.Error(), "access denied") {
			errorResp := &response.ErrorResponse{
				Code:             response.ErrAccessDenied.Code,
				ErrorDescription: err.Error(),
			}
			response.RespondWithError(c, response.StatusForbidden, errorResp)
		} else if strings.Contains(err.Error(), "invalid") {
			errorResp := &response.ErrorResponse{
				Code:             response.ErrInvalidRequest.Code,
				ErrorDescription: err.Error(),
			}
			response.RespondWithError(c, response.StatusBadRequest, errorResp)
		} else {
			errorResp := &response.ErrorResponse{
				Code:             response.ErrServerError.Code,
				ErrorDescription: "Failed to retrieve team members",
			}
			response.RespondWithError(c, response.StatusInternalServerError, errorResp)
		}
		return
	}

	// Return the paginated result
	response.RespondWithSuccess(c, http.StatusOK, result)
}

// GinMemberCheckRobotEmail handles GET /api/user/teams/:id/members/check-robot-email?robot_email=xxx - Check if robot email exists globally
func GinMemberCheckRobotEmail(c *gin.Context) {
	// Get authorized user info
	authInfo := oauth.GetAuthorizedInfo(c)
	if authInfo == nil || authInfo.UserID == "" {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidClient.Code,
			ErrorDescription: "User not authenticated",
		}
		response.RespondWithError(c, response.StatusUnauthorized, errorResp)
		return
	}

	teamID := c.Param("id")
	if teamID == "" {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidRequest.Code,
			ErrorDescription: "Team ID is required",
		}
		response.RespondWithError(c, response.StatusBadRequest, errorResp)
		return
	}

	robotEmail := c.Query("robot_email")
	if robotEmail == "" {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidRequest.Code,
			ErrorDescription: "robot_email query parameter is required",
		}
		response.RespondWithError(c, response.StatusBadRequest, errorResp)
		return
	}

	// Call business logic
	exists, err := memberCheckRobotEmail(c.Request.Context(), authInfo.UserID, teamID, robotEmail)
	if err != nil {
		log.Error("Failed to check robot email: %v", err)
		// Check error type for appropriate response
		if strings.Contains(err.Error(), "not found") {
			errorResp := &response.ErrorResponse{
				Code:             response.ErrInvalidRequest.Code,
				ErrorDescription: "Team not found",
			}
			response.RespondWithError(c, response.StatusNotFound, errorResp)
		} else if strings.Contains(err.Error(), "access denied") {
			errorResp := &response.ErrorResponse{
				Code:             response.ErrAccessDenied.Code,
				ErrorDescription: err.Error(),
			}
			response.RespondWithError(c, response.StatusForbidden, errorResp)
		} else {
			errorResp := &response.ErrorResponse{
				Code:             response.ErrServerError.Code,
				ErrorDescription: fmt.Sprintf("Failed to check robot email: %v", err),
			}
			response.RespondWithError(c, response.StatusInternalServerError, errorResp)
		}
		return
	}

	// Return result
	result := map[string]interface{}{
		"exists":      exists,
		"robot_email": robotEmail,
	}
	response.RespondWithSuccess(c, http.StatusOK, result)
}

// GinMemberGet handles GET /teams/:team_id/members/:member_id - Get team member details
func GinMemberGet(c *gin.Context) {
	// Get authorized user info
	authInfo := oauth.GetAuthorizedInfo(c)
	if authInfo == nil || authInfo.UserID == "" {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidClient.Code,
			ErrorDescription: "User not authenticated",
		}
		response.RespondWithError(c, response.StatusUnauthorized, errorResp)
		return
	}

	teamID := c.Param("id")
	memberID := c.Param("member_id")
	if teamID == "" || memberID == "" {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidRequest.Code,
			ErrorDescription: "Team ID and Member ID are required",
		}
		response.RespondWithError(c, response.StatusBadRequest, errorResp)
		return
	}

	// Call business logic
	memberData, err := memberGet(c.Request.Context(), authInfo.UserID, teamID, memberID)
	if err != nil {
		log.Error("Failed to get member details: %v", err)
		// Check error type for appropriate response
		if strings.Contains(err.Error(), "not found") {
			errorResp := &response.ErrorResponse{
				Code:             response.ErrInvalidRequest.Code,
				ErrorDescription: "Member not found",
			}
			response.RespondWithError(c, response.StatusNotFound, errorResp)
		} else if strings.Contains(err.Error(), "access denied") {
			errorResp := &response.ErrorResponse{
				Code:             response.ErrAccessDenied.Code,
				ErrorDescription: err.Error(),
			}
			response.RespondWithError(c, response.StatusForbidden, errorResp)
		} else {
			errorResp := &response.ErrorResponse{
				Code:             response.ErrServerError.Code,
				ErrorDescription: "Failed to retrieve member details",
			}
			response.RespondWithError(c, response.StatusInternalServerError, errorResp)
		}
		return
	}

	// Convert to response format
	member := mapToMemberDetailResponse(memberData)
	response.RespondWithSuccess(c, http.StatusOK, member)
}

// GinMemberCreateRobot handles POST /teams/:team_id/members/robots - Add robot member to team
func GinMemberCreateRobot(c *gin.Context) {
	// Get authorized user info
	authInfo := authorized.GetInfo(c)
	if authInfo == nil || authInfo.UserID == "" {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidClient.Code,
			ErrorDescription: "User not authenticated",
		}
		response.RespondWithError(c, response.StatusUnauthorized, errorResp)
		return
	}

	teamID := c.Param("id")
	if teamID == "" {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidRequest.Code,
			ErrorDescription: "Team ID is required",
		}
		response.RespondWithError(c, response.StatusBadRequest, errorResp)
		return
	}

	// Parse request body
	var req CreateRobotMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidRequest.Code,
			ErrorDescription: "Invalid request body: " + err.Error(),
		}
		response.RespondWithError(c, response.StatusBadRequest, errorResp)
		return
	}

	// Prepare base robot member data
	baseData := maps.MapStrAny{
		"display_name":    req.Name,
		"robot_email":     req.RobotEmail, // Required: globally unique email
		"bio":             req.Bio,
		"role_id":         req.RoleID,
		"system_prompt":   req.SystemPrompt,
		"autonomous_mode": toBool(req.AutonomousMode),
	}

	// Add optional fields
	if req.Email != "" {
		baseData["email"] = req.Email // Optional: display-only email
	}
	if len(req.AuthorizedSenders) > 0 {
		baseData["authorized_senders"] = req.AuthorizedSenders
	}
	if len(req.EmailFilterRules) > 0 {
		baseData["email_filter_rules"] = req.EmailFilterRules
	}
	if req.ManagerID != "" {
		baseData["manager_id"] = req.ManagerID
	}
	if req.LanguageModel != "" {
		baseData["language_model"] = req.LanguageModel
	}
	if len(req.Agents) > 0 {
		baseData["agents"] = req.Agents
	}
	if len(req.MCPServers) > 0 {
		baseData["mcp_servers"] = req.MCPServers
	}
	if req.CostLimit > 0 {
		baseData["cost_limit"] = req.CostLimit
	}

	// Wrap with create scope for permission tracking
	robotData := authInfo.WithCreateScope(baseData)

	// Call business logic
	memberID, err := memberCreateRobot(c.Request.Context(), authInfo.UserID, teamID, robotData)
	if err != nil {
		log.Error("Failed to create robot member: %v", err)
		// Check error type for appropriate response
		if strings.Contains(err.Error(), "not found") {
			errorResp := &response.ErrorResponse{
				Code:             response.ErrInvalidRequest.Code,
				ErrorDescription: "Team not found",
			}
			response.RespondWithError(c, response.StatusNotFound, errorResp)
		} else if strings.Contains(err.Error(), "access denied") {
			errorResp := &response.ErrorResponse{
				Code:             response.ErrAccessDenied.Code,
				ErrorDescription: err.Error(),
			}
			response.RespondWithError(c, response.StatusForbidden, errorResp)
		} else if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "duplicate") {
			errorResp := &response.ErrorResponse{
				Code:             response.ErrInvalidRequest.Code,
				ErrorDescription: err.Error(),
			}
			response.RespondWithError(c, response.StatusConflict, errorResp)
		} else {
			errorResp := &response.ErrorResponse{
				Code:             response.ErrServerError.Code,
				ErrorDescription: "Failed to create robot member",
			}
			response.RespondWithError(c, response.StatusInternalServerError, errorResp)
		}
		return
	}

	// Return created member ID
	response.RespondWithSuccess(c, http.StatusCreated, gin.H{"member_id": memberID})
}

// GinMemberUpdate handles PUT /teams/:team_id/members/:member_id - Update team member
func GinMemberUpdate(c *gin.Context) {
	// Get authorized user info
	authInfo := oauth.GetAuthorizedInfo(c)
	if authInfo == nil || authInfo.UserID == "" {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidClient.Code,
			ErrorDescription: "User not authenticated",
		}
		response.RespondWithError(c, response.StatusUnauthorized, errorResp)
		return
	}

	teamID := c.Param("id")
	memberID := c.Param("member_id")
	if teamID == "" || memberID == "" {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidRequest.Code,
			ErrorDescription: "Team ID and Member ID are required",
		}
		response.RespondWithError(c, response.StatusBadRequest, errorResp)
		return
	}

	// Parse request body
	var req UpdateMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidRequest.Code,
			ErrorDescription: "Invalid request body: " + err.Error(),
		}
		response.RespondWithError(c, response.StatusBadRequest, errorResp)
		return
	}

	// Prepare update data
	updateData := maps.MapStrAny{}

	if req.RoleID != "" {
		updateData["role_id"] = req.RoleID
	}
	if req.Status != "" {
		updateData["status"] = req.Status
	}
	if req.Settings != nil {
		updateData["settings"] = req.Settings
	}
	if req.LastActivity != "" {
		updateData["last_activity"] = req.LastActivity
	}

	// Call business logic
	err := memberUpdate(c.Request.Context(), authInfo.UserID, teamID, memberID, updateData)
	if err != nil {
		log.Error("Failed to update member: %v", err)
		// Check error type for appropriate response
		if strings.Contains(err.Error(), "not found") {
			errorResp := &response.ErrorResponse{
				Code:             response.ErrInvalidRequest.Code,
				ErrorDescription: "Member not found",
			}
			response.RespondWithError(c, response.StatusNotFound, errorResp)
		} else if strings.Contains(err.Error(), "access denied") {
			errorResp := &response.ErrorResponse{
				Code:             response.ErrAccessDenied.Code,
				ErrorDescription: err.Error(),
			}
			response.RespondWithError(c, response.StatusForbidden, errorResp)
		} else {
			errorResp := &response.ErrorResponse{
				Code:             response.ErrServerError.Code,
				ErrorDescription: "Failed to update member",
			}
			response.RespondWithError(c, response.StatusInternalServerError, errorResp)
		}
		return
	}

	response.RespondWithSuccess(c, http.StatusOK, gin.H{"message": "Member updated successfully"})
}

// GinMemberDelete handles DELETE /teams/:team_id/members/:member_id - Remove team member
func GinMemberDelete(c *gin.Context) {
	// Get authorized user info
	authInfo := oauth.GetAuthorizedInfo(c)
	if authInfo == nil || authInfo.UserID == "" {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidClient.Code,
			ErrorDescription: "User not authenticated",
		}
		response.RespondWithError(c, response.StatusUnauthorized, errorResp)
		return
	}

	teamID := c.Param("id")
	memberID := c.Param("member_id")
	if teamID == "" || memberID == "" {
		errorResp := &response.ErrorResponse{
			Code:             response.ErrInvalidRequest.Code,
			ErrorDescription: "Team ID and Member ID are required",
		}
		response.RespondWithError(c, response.StatusBadRequest, errorResp)
		return
	}

	// Call business logic
	err := memberDelete(c.Request.Context(), authInfo.UserID, teamID, memberID)
	if err != nil {
		log.Error("Failed to delete member: %v", err)
		// Check error type for appropriate response
		if strings.Contains(err.Error(), "not found") {
			errorResp := &response.ErrorResponse{
				Code:             response.ErrInvalidRequest.Code,
				ErrorDescription: "Member not found",
			}
			response.RespondWithError(c, response.StatusNotFound, errorResp)
		} else if strings.Contains(err.Error(), "access denied") {
			errorResp := &response.ErrorResponse{
				Code:             response.ErrAccessDenied.Code,
				ErrorDescription: err.Error(),
			}
			response.RespondWithError(c, response.StatusForbidden, errorResp)
		} else {
			errorResp := &response.ErrorResponse{
				Code:             response.ErrServerError.Code,
				ErrorDescription: "Failed to delete member",
			}
			response.RespondWithError(c, response.StatusInternalServerError, errorResp)
		}
		return
	}

	response.RespondWithSuccess(c, http.StatusOK, gin.H{"message": "Member removed successfully"})
}

// Yao Process Handlers (for Yao application calls)

// ProcessMemberList user.member.list Member list processor
// Args[0] string: team_id
// Args[1] map: Query parameters with advanced filtering
//
//	{
//	  "page": 1, "pagesize": 20,
//	  "status": "active", "member_type": "user", "role_id": "admin",
//	  "email": "test@example.com", "display_name": "John",
//	  "order": "created_at desc",
//	  "fields": ["id", "user_id", "display_name", "role_id"]
//	}
//
// Return: map: Paginated member list
func ProcessMemberList(process *process.Process) interface{} {
	process.ValidateArgNums(2)

	// Get user_id from session
	userIDStr := GetUserIDFromSession(process)

	teamID := process.ArgsString(0)
	if teamID == "" {
		exception.New("team_id is required", 400).Throw()
	}

	// Parse query parameters
	queryMap := process.ArgsMap(1)

	// Build request object
	req := &MemberListRequest{
		Page:     1,
		PageSize: 20,
		Order:    "created_at desc",
	}

	// Parse pagination
	if p, ok := queryMap["page"]; ok {
		if pageInt, ok := p.(int); ok && pageInt > 0 {
			req.Page = pageInt
		}
	}

	if ps, ok := queryMap["pagesize"]; ok {
		if pagesizeInt, ok := ps.(int); ok && pagesizeInt > 0 && pagesizeInt <= 100 {
			req.PageSize = pagesizeInt
		}
	}

	// Parse filters
	if status, ok := queryMap["status"].(string); ok {
		req.Status = status
	}

	if memberType, ok := queryMap["member_type"].(string); ok {
		req.MemberType = memberType
	}

	if roleID, ok := queryMap["role_id"].(string); ok {
		req.RoleID = roleID
	}

	if email, ok := queryMap["email"].(string); ok {
		req.Email = email
	}

	if displayName, ok := queryMap["display_name"].(string); ok {
		req.DisplayName = displayName
	}

	// Parse sorting
	if order, ok := queryMap["order"].(string); ok {
		req.Order = order
	}

	// Parse fields selection
	if fields, ok := queryMap["fields"]; ok {
		if fieldsSlice, ok := fields.([]interface{}); ok {
			req.Fields = make([]string, 0, len(fieldsSlice))
			for _, f := range fieldsSlice {
				if fieldStr, ok := f.(string); ok {
					req.Fields = append(req.Fields, fieldStr)
				}
			}
		} else if fieldsStrSlice, ok := fields.([]string); ok {
			req.Fields = fieldsStrSlice
		}
	}

	// Get context
	ctx := process.Context
	if ctx == nil {
		ctx = context.Background()
	}

	// Call business logic
	result, err := memberList(ctx, userIDStr, teamID, req)
	if err != nil {
		exception.New("failed to list members: %s", 500, err.Error()).Throw()
	}

	return result
}

// ProcessMemberGet user.member.get Member get processor
// Args[0] string: team_id
// Args[1] string: member_id
// Return: map: Member details
func ProcessMemberGet(process *process.Process) interface{} {
	process.ValidateArgNums(2)

	// Get user_id from session
	userIDStr := GetUserIDFromSession(process)

	teamID := process.ArgsString(0)
	memberID := process.ArgsString(1)

	if teamID == "" || memberID == "" {
		exception.New("team_id and member_id are required", 400).Throw()
	}

	// Get context
	ctx := process.Context
	if ctx == nil {
		ctx = context.Background()
	}

	// Call business logic
	result, err := memberGet(ctx, userIDStr, teamID, memberID)
	if err != nil {
		exception.New("failed to get member: %s", 500, err.Error()).Throw()
	}

	return result
}

// ProcessMemberUpdate user.member.update Member update processor
// Args[0] string: team_id
// Args[1] string: member_id
// Args[2] map: Update data {"role_id": "admin", "status": "active", "settings": {...}}
// Return: map: {"message": "success"}
func ProcessMemberUpdate(process *process.Process) interface{} {
	process.ValidateArgNums(3)

	// Get user_id from session
	userIDStr := GetUserIDFromSession(process)

	teamID := process.ArgsString(0)
	memberID := process.ArgsString(1)
	updateData := maps.MapStrAny(process.ArgsMap(2))

	if teamID == "" || memberID == "" {
		exception.New("team_id and member_id are required", 400).Throw()
	}

	// Get context
	ctx := process.Context
	if ctx == nil {
		ctx = context.Background()
	}

	// Call business logic
	err := memberUpdate(ctx, userIDStr, teamID, memberID, updateData)
	if err != nil {
		exception.New("failed to update member: %s", 500, err.Error()).Throw()
	}

	return map[string]interface{}{
		"message": "success",
	}
}

// ProcessMemberDelete user.member.delete Member delete processor
// Args[0] string: team_id
// Args[1] string: member_id
// Return: map: {"message": "success"}
func ProcessMemberDelete(process *process.Process) interface{} {
	process.ValidateArgNums(2)

	// Get user_id from session
	userIDStr := GetUserIDFromSession(process)

	teamID := process.ArgsString(0)
	memberID := process.ArgsString(1)

	if teamID == "" || memberID == "" {
		exception.New("team_id and member_id are required", 400).Throw()
	}

	// Get context
	ctx := process.Context
	if ctx == nil {
		ctx = context.Background()
	}

	// Call business logic
	err := memberDelete(ctx, userIDStr, teamID, memberID)
	if err != nil {
		exception.New("failed to delete member: %s", 500, err.Error()).Throw()
	}

	return map[string]interface{}{
		"message": "success",
	}
}

// Private Business Logic Functions (internal use only)

// memberList handles the business logic for listing team members with advanced filtering
func memberList(ctx context.Context, userID, teamID string, req *MemberListRequest) (maps.MapStr, error) {
	// Check if user has access to the team (read permission: owner or member)
	isOwner, isMember, err := checkTeamAccess(ctx, teamID, userID)
	if err != nil {
		return nil, err
	}

	// Allow access if user is owner or member
	if !isOwner && !isMember {
		return nil, fmt.Errorf("access denied: user is not a member of this team")
	}

	// Get user provider instance
	provider, err := getUserProvider()
	if err != nil {
		return nil, fmt.Errorf("failed to get user provider: %w", err)
	}

	// Build query parameters
	param := model.QueryParam{
		Wheres: []model.QueryWhere{
			{Column: "team_id", Value: teamID},
		},
	}

	// Add filters
	if req.Status != "" {
		// Validate status values
		validStatuses := map[string]bool{
			"pending": true, "active": true, "inactive": true, "suspended": true,
		}
		if !validStatuses[req.Status] {
			return nil, fmt.Errorf("invalid status value: %s (must be one of: pending, active, inactive, suspended)", req.Status)
		}
		param.Wheres = append(param.Wheres, model.QueryWhere{
			Column: "status",
			Value:  req.Status,
		})
	}

	if req.MemberType != "" {
		// Validate member type values
		validTypes := map[string]bool{
			"user": true, "robot": true,
		}
		if !validTypes[req.MemberType] {
			return nil, fmt.Errorf("invalid member_type value: %s (must be one of: user, robot)", req.MemberType)
		}
		param.Wheres = append(param.Wheres, model.QueryWhere{
			Column: "member_type",
			Value:  req.MemberType,
		})
	}

	if req.RoleID != "" {
		param.Wheres = append(param.Wheres, model.QueryWhere{
			Column: "role_id",
			Value:  req.RoleID,
		})
	}

	if req.Email != "" {
		param.Wheres = append(param.Wheres, model.QueryWhere{
			Column: "email",
			Value:  req.Email,
		})
	}

	if req.DisplayName != "" {
		param.Wheres = append(param.Wheres, model.QueryWhere{
			Column: "display_name",
			Value:  req.DisplayName,
			OP:     "like",
		})
	}

	// Parse and validate sorting
	validOrderFields := map[string]bool{
		"created_at": true,
		"joined_at":  true,
	}
	validOrderDirs := map[string]bool{
		"asc": true, "desc": true,
	}

	// Parse order field (format: "field_name [asc|desc]")
	orderParts := strings.Fields(req.Order) // Split by whitespace
	orderBy := ""
	orderDir := "desc" // Default direction

	if len(orderParts) > 0 {
		orderBy = orderParts[0]
		if len(orderParts) > 1 {
			orderDir = strings.ToLower(orderParts[1])
		}
	}

	// Build sorting with priority: owner first, then pending invitations, then others
	orders := []model.QueryOrder{
		{Column: "is_owner", Option: "desc"}, // Owners always first
		{Column: "status", Option: "asc"},    // Then pending before active (enum index: pending=1 < active=2 < inactive=3 < suspended=4)
	}

	// Validate and add user-specified order field
	if orderBy != "" {
		if !validOrderFields[orderBy] {
			return nil, fmt.Errorf("invalid order field: %s (must be one of: created_at, joined_at)", orderBy)
		}
		if !validOrderDirs[orderDir] {
			return nil, fmt.Errorf("invalid order direction: %s (must be one of: asc, desc)", orderDir)
		}
		orders = append(orders, model.QueryOrder{
			Column: orderBy, Option: orderDir,
		})
	} else {
		// Default tertiary sorting
		orders = append(orders, model.QueryOrder{
			Column: "created_at", Option: "desc",
		})
	}

	param.Orders = orders

	// Add field selection if specified
	if len(req.Fields) > 0 {
		// Convert []string to []interface{} for QueryParam.Select
		param.Select = make([]interface{}, len(req.Fields))
		for i, field := range req.Fields {
			param.Select[i] = field
		}
	}

	// Get paginated members
	result, err := provider.PaginateMembers(ctx, param, req.Page, req.PageSize)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve members: %w", err)
	}

	return result, nil
}

// memberGet handles the business logic for getting a specific team member
func memberGet(ctx context.Context, userID, teamID, memberID string) (maps.MapStrAny, error) {
	// Check if user has access to the team (read permission: owner or member)
	isOwner, isMember, err := checkTeamAccess(ctx, teamID, userID)
	if err != nil {
		return nil, err
	}

	// Allow access if user is owner or member
	if !isOwner && !isMember {
		return nil, fmt.Errorf("access denied: user is not a member of this team")
	}

	// Get user provider instance
	provider, err := getUserProvider()
	if err != nil {
		return nil, fmt.Errorf("failed to get user provider: %w", err)
	}

	// Get member details using member_id (with all fields including robot config)
	memberData, err := provider.GetMemberDetailByMemberID(ctx, memberID)
	if err != nil {
		return nil, fmt.Errorf("member not found: %w", err)
	}

	return memberData, nil
}

// memberCheckRobotEmail handles the business logic for checking if robot email exists globally
func memberCheckRobotEmail(ctx context.Context, userID, teamID, robotEmail string) (bool, error) {
	// Check if user has access to the team (read permission: owner or member)
	isOwner, isMember, err := checkTeamAccess(ctx, teamID, userID)
	if err != nil {
		return false, err
	}

	// Allow access if user is owner or member
	if !isOwner && !isMember {
		return false, fmt.Errorf("access denied: user is not a member of this team")
	}

	// Get user provider instance
	provider, err := getUserProvider()
	if err != nil {
		return false, fmt.Errorf("failed to get user provider: %w", err)
	}

	// Check if robot email exists globally (not limited to team)
	exists, err := provider.MemberExistsByRobotEmail(ctx, robotEmail)
	if err != nil {
		return false, fmt.Errorf("failed to check robot email existence: %w", err)
	}

	return exists, nil
}

// memberCreateRobot handles the business logic for creating a robot member
func memberCreateRobot(ctx context.Context, userID, teamID string, robotData maps.MapStrAny) (string, error) {
	// Check if user has access to the team (write permission: owner only)
	isOwner, _, err := checkTeamAccess(ctx, teamID, userID)
	if err != nil {
		return "", err
	}

	// Only allow access if user is owner
	if !isOwner {
		return "", fmt.Errorf("access denied: only team owner can add robot members")
	}

	// Get user provider instance
	provider, err := getUserProvider()
	if err != nil {
		return "", fmt.Errorf("failed to get user provider: %w", err)
	}

	// Use CreateRobotMember method which handles robot-specific logic
	memberID, err := provider.CreateRobotMember(ctx, teamID, robotData)
	if err != nil {
		return "", fmt.Errorf("failed to create robot member: %w", err)
	}

	return memberID, nil
}

// memberUpdate handles the business logic for updating a team member
func memberUpdate(ctx context.Context, userID, teamID, memberID string, updateData maps.MapStrAny) error {
	// Check if user has access to the team (write permission: owner only)
	isOwner, _, err := checkTeamAccess(ctx, teamID, userID)
	if err != nil {
		return err
	}

	// Only allow access if user is owner
	if !isOwner {
		return fmt.Errorf("access denied: only team owner can update members")
	}

	// Get user provider instance
	provider, err := getUserProvider()
	if err != nil {
		return fmt.Errorf("failed to get user provider: %w", err)
	}

	// Check if member exists using member_id
	_, err = provider.GetMemberByMemberID(ctx, memberID)
	if err != nil {
		return fmt.Errorf("member not found: %w", err)
	}

	// Add updated_at timestamp
	updateData["updated_at"] = time.Now()

	// Update member using member_id
	err = provider.UpdateMemberByMemberID(ctx, memberID, updateData)
	if err != nil {
		return fmt.Errorf("failed to update member: %w", err)
	}

	return nil
}

// memberDelete handles the business logic for deleting a team member
func memberDelete(ctx context.Context, userID, teamID, memberID string) error {
	// Check if user has access to the team (write permission: owner only)
	isOwner, _, err := checkTeamAccess(ctx, teamID, userID)
	if err != nil {
		return err
	}

	// Only allow access if user is owner
	if !isOwner {
		return fmt.Errorf("access denied: only team owner can remove members")
	}

	// Get user provider instance
	provider, err := getUserProvider()
	if err != nil {
		return fmt.Errorf("failed to get user provider: %w", err)
	}

	// Check if member exists using member_id
	_, err = provider.GetMemberByMemberID(ctx, memberID)
	if err != nil {
		return fmt.Errorf("member not found: %w", err)
	}

	// Remove member using member_id
	err = provider.RemoveMemberByMemberID(ctx, memberID)
	if err != nil {
		return fmt.Errorf("failed to delete member: %w", err)
	}

	return nil
}

// Private Helper Functions (internal use only)

// checkTeamAccess checks if user has access to the team
// Returns: (isOwner bool, isMember bool, error)
func checkTeamAccess(ctx context.Context, teamID, userID string) (bool, bool, error) {
	// Get user provider instance
	provider, err := getUserProvider()
	if err != nil {
		return false, false, fmt.Errorf("failed to get user provider: %w", err)
	}

	// Use UserProvider's CheckTeamAccess method - note parameter order: (ctx, teamID, userID)
	return provider.CheckTeamAccess(ctx, teamID, userID)
}

// mapToMemberResponse converts a map to MemberResponse
func mapToMemberResponse(data maps.MapStr) MemberResponse {
	member := MemberResponse{
		ID:                  toInt64(data["id"]),
		MemberID:            toString(data["member_id"]),
		TeamID:              toString(data["team_id"]),
		UserID:              toString(data["user_id"]),
		MemberType:          toString(data["member_type"]),
		DisplayName:         toString(data["display_name"]),
		Bio:                 toString(data["bio"]),
		Avatar:              toString(data["avatar"]),
		Email:               toString(data["email"]),
		RobotEmail:          toString(data["robot_email"]), // Globally unique email for robot members
		RoleID:              toString(data["role_id"]),
		IsOwner:             data["is_owner"], // Keep original type (int or bool)
		Status:              toString(data["status"]),
		InvitationID:        toString(data["invitation_id"]),
		InvitedBy:           toString(data["invited_by"]),
		InvitedAt:           toTimeString(data["invited_at"]),
		InvitationToken:     toString(data["invitation_token"]),
		InvitationExpiresAt: toTimeString(data["invitation_expires_at"]),
		JoinedAt:            toTimeString(data["joined_at"]),
		LastActiveAt:        toTimeString(data["last_active_at"]),
		LoginCount:          toInt(data["login_count"]),
		CreatedAt:           toTimeString(data["created_at"]),
		UpdatedAt:           toTimeString(data["updated_at"]),
	}

	// Add settings if available
	if settings, ok := data["settings"]; ok {
		if memSettings, ok := settings.(*MemberSettings); ok {
			member.Settings = memSettings
		} else if settingsMap, ok := settings.(map[string]interface{}); ok {
			// Convert map to MemberSettings (for backward compatibility)
			memSettings := &MemberSettings{
				Notifications: toBool(settingsMap["notifications"]),
			}
			// Handle permissions array
			if perms, ok := settingsMap["permissions"]; ok {
				if permsSlice, ok := perms.([]interface{}); ok {
					permissions := make([]string, 0, len(permsSlice))
					for _, p := range permsSlice {
						if permStr, ok := p.(string); ok {
							permissions = append(permissions, permStr)
						}
					}
					memSettings.Permissions = permissions
				} else if permsStrSlice, ok := perms.([]string); ok {
					memSettings.Permissions = permsStrSlice
				}
			}
			member.Settings = memSettings
		}
	}

	return member
}

// mapToMemberDetailResponse converts a map to MemberDetailResponse
func mapToMemberDetailResponse(data maps.MapStr) MemberDetailResponse {
	member := MemberDetailResponse{
		MemberResponse: mapToMemberResponse(data),
		// Robot-specific fields
		SystemPrompt:      toString(data["system_prompt"]),
		ManagerID:         toString(data["manager_id"]),
		LanguageModel:     toString(data["language_model"]),
		CostLimit:         toFloat64(data["cost_limit"]),
		AutonomousMode:    data["autonomous_mode"], // Keep original type (bool or string)
		LastRobotActivity: toTimeString(data["last_robot_activity"]),
		RobotStatus:       toString(data["robot_status"]),
		Notes:             toString(data["notes"]),
	}

	// Handle authorized_senders array
	if authorizedSenders, ok := data["authorized_senders"]; ok {
		if sendersSlice, ok := authorizedSenders.([]interface{}); ok {
			sendersList := make([]string, 0, len(sendersSlice))
			for _, s := range sendersSlice {
				if senderStr, ok := s.(string); ok {
					sendersList = append(sendersList, senderStr)
				}
			}
			member.AuthorizedSenders = sendersList
		} else if sendersStrSlice, ok := authorizedSenders.([]string); ok {
			member.AuthorizedSenders = sendersStrSlice
		}
	}

	// Handle email_filter_rules array
	if filterRules, ok := data["email_filter_rules"]; ok {
		if rulesSlice, ok := filterRules.([]interface{}); ok {
			rulesList := make([]string, 0, len(rulesSlice))
			for _, r := range rulesSlice {
				if ruleStr, ok := r.(string); ok {
					rulesList = append(rulesList, ruleStr)
				}
			}
			member.EmailFilterRules = rulesList
		} else if rulesStrSlice, ok := filterRules.([]string); ok {
			member.EmailFilterRules = rulesStrSlice
		}
	}

	// Handle robot_config map
	if robotConfig, ok := data["robot_config"]; ok {
		if configMap, ok := robotConfig.(map[string]interface{}); ok {
			member.RobotConfig = configMap
		}
	}

	// Handle agents array
	if agents, ok := data["agents"]; ok {
		if agentsSlice, ok := agents.([]interface{}); ok {
			agentsList := make([]string, 0, len(agentsSlice))
			for _, a := range agentsSlice {
				if agentStr, ok := a.(string); ok {
					agentsList = append(agentsList, agentStr)
				}
			}
			member.Agents = agentsList
		} else if agentsStrSlice, ok := agents.([]string); ok {
			member.Agents = agentsStrSlice
		}
	}

	// Handle mcp_servers array
	if mcpServers, ok := data["mcp_servers"]; ok {
		if serversSlice, ok := mcpServers.([]interface{}); ok {
			serversList := make([]string, 0, len(serversSlice))
			for _, s := range serversSlice {
				if serverStr, ok := s.(string); ok {
					serversList = append(serversList, serverStr)
				}
			}
			member.MCPServers = serversList
		} else if serversStrSlice, ok := mcpServers.([]string); ok {
			member.MCPServers = serversStrSlice
		}
	}

	// Handle metadata map
	if metadata, ok := data["metadata"]; ok {
		if metadataMap, ok := metadata.(map[string]interface{}); ok {
			member.Metadata = metadataMap
		}
	}

	// Add user info if available (could be joined from user table)
	if userInfo, ok := data["user_info"]; ok {
		if userInfoMap, ok := userInfo.(map[string]interface{}); ok {
			member.UserInfo = userInfoMap
		}
	}

	return member
}
