package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/Anything-That-Works/GoPath/internal/cache"
	"github.com/Anything-That-Works/GoPath/internal/database"
	"github.com/Anything-That-Works/GoPath/internal/model"
	"github.com/Anything-That-Works/GoPath/internal/ws"
	"github.com/google/uuid"
)

func (apiConfig *apiConfig) handlerCreateConversation(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		IsGroup bool        `json:"is_group"`
		Name    *string     `json:"name"`
		Members []uuid.UUID `json:"members"`
	}

	userID, ok := r.Context().Value(contextKeyUserID).(uuid.UUID)
	if !ok {
		respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Unauthorized"})
		return
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Invalid request payload"})
		return
	}

	if len(params.Members) == 0 {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "At least one member required"})
		return
	}

	if params.IsGroup && (params.Name == nil || *params.Name == "") {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Group name required"})
		return
	}

	if !params.IsGroup && len(params.Members) != 1 {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Direct message must have exactly one member"})
		return
	}

	if !params.IsGroup {
		existing, err := apiConfig.DB.GetDirectConversation(r.Context(), database.GetDirectConversationParams{
			UserID:   userID,
			UserID_2: params.Members[0],
		})
		if err == nil {
			respondWithJSON(w, 200, model.APIResponse{
				Success: true,
				Message: "Conversation already exists",
				Data:    existing,
			})
			return
		}
		if err != sql.ErrNoRows {
			respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to check existing conversation"})
			return
		}
	}

	var name sql.NullString
	if params.Name != nil {
		name = sql.NullString{String: *params.Name, Valid: true}
	}

	conversation, err := apiConfig.DB.CreateConversation(r.Context(), database.CreateConversationParams{
		CreatedBy: userID,
		IsGroup:   params.IsGroup,
		Name:      name,
	})
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to create conversation"})
		return
	}

	// creator is super_admin for groups, member for direct
	creatorRole := database.MemberRoleMember
	if params.IsGroup {
		creatorRole = database.MemberRoleSuperAdmin
	}

	err = apiConfig.DB.AddConversationMember(r.Context(), database.AddConversationMemberParams{
		ConversationID: conversation.ID,
		UserID:         userID,
		Role:           creatorRole,
	})
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to add creator to conversation"})
		return
	}

	for _, memberID := range params.Members {
		err = apiConfig.DB.AddConversationMember(r.Context(), database.AddConversationMemberParams{
			ConversationID: conversation.ID,
			UserID:         memberID,
			Role:           database.MemberRoleMember,
		})
		if err != nil {
			respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to add member to conversation"})
			return
		}
	}

	if err := apiConfig.Cache.DeleteByPattern(r.Context(), fmt.Sprintf("conversations:list:%s:*", userID.String())); err != nil {
		log.Printf("Failed to invalidate conversations cache: %v", err)
	}

	respondWithJSON(w, 201, model.APIResponse{
		Success: true,
		Message: "Conversation created successfully",
		Data:    conversation,
	})
}

func (apiConfig *apiConfig) handlerAddMember(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(contextKeyUserID).(uuid.UUID)
	if !ok {
		respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Unauthorized"})
		return
	}

	type parameters struct {
		ConversationID uuid.UUID `json:"conversation_id"`
		UserID         uuid.UUID `json:"user_id"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Invalid request payload"})
		return
	}

	conversation, err := apiConfig.DB.GetConversationByID(r.Context(), params.ConversationID)
	if err != nil {
		respondWithJSON(w, 404, model.APIResponse{Success: false, Message: "Conversation not found"})
		return
	}
	if !conversation.IsGroup {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Cannot add members to a direct conversation"})
		return
	}

	member, err := apiConfig.DB.GetConversationMember(r.Context(), database.GetConversationMemberParams{
		ConversationID: params.ConversationID,
		UserID:         userID,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			respondWithJSON(w, 403, model.APIResponse{Success: false, Message: "Not a member of this conversation"})
			return
		}
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to verify membership"})
		return
	}

	if member.Role == database.MemberRoleMember {
		respondWithJSON(w, 403, model.APIResponse{Success: false, Message: "Only admins can add members"})
		return
	}

	err = apiConfig.DB.AddConversationMember(r.Context(), database.AddConversationMemberParams{
		ConversationID: params.ConversationID,
		UserID:         params.UserID,
		Role:           database.MemberRoleMember,
	})
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to add member"})
		return
	}

	if err := apiConfig.Cache.Delete(r.Context(), cache.KeyConversationMembers(params.ConversationID.String())); err != nil {
		log.Printf("Failed to invalidate conversation members cache: %v", err)
	}

	respondWithJSON(w, 200, model.APIResponse{
		Success: true,
		Message: "Member added successfully",
	})
}

func (apiConfig *apiConfig) handlerRemoveMember(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(contextKeyUserID).(uuid.UUID)
	if !ok {
		respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Unauthorized"})
		return
	}

	type parameters struct {
		ConversationID uuid.UUID `json:"conversation_id"`
		UserID         uuid.UUID `json:"user_id"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Invalid request payload"})
		return
	}

	requester, err := apiConfig.DB.GetConversationMember(r.Context(), database.GetConversationMemberParams{
		ConversationID: params.ConversationID,
		UserID:         userID,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			respondWithJSON(w, 403, model.APIResponse{Success: false, Message: "Not a member of this conversation"})
			return
		}
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to verify membership"})
		return
	}

	// get target member
	target, err := apiConfig.DB.GetConversationMember(r.Context(), database.GetConversationMemberParams{
		ConversationID: params.ConversationID,
		UserID:         params.UserID,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			respondWithJSON(w, 404, model.APIResponse{Success: false, Message: "Member not found"})
			return
		}
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to fetch member"})
		return
	}

	isSelf := userID == params.UserID

	switch requester.Role {
	case database.MemberRoleMember:
		// members can only remove themselves
		if !isSelf {
			respondWithJSON(w, 403, model.APIResponse{Success: false, Message: "Only admins can remove members"})
			return
		}
	case database.MemberRoleAdmin:
		// admins can remove members or themselves, but not super_admin or other admins
		if !isSelf && target.Role != database.MemberRoleMember {
			respondWithJSON(w, 403, model.APIResponse{Success: false, Message: "Admins can only remove regular members"})
			return
		}
	case database.MemberRoleSuperAdmin:
		// super admin can remove anyone including themselves
	}

	err = apiConfig.DB.RemoveConversationMember(r.Context(), database.RemoveConversationMemberParams{
		ConversationID: params.ConversationID,
		UserID:         params.UserID,
	})
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to remove member"})
		return
	}

	// if super admin removed themselves, assign new super admin
	if isSelf && requester.Role == database.MemberRoleSuperAdmin {
		next, err := apiConfig.DB.GetFirstAdminOrMember(r.Context(), database.GetFirstAdminOrMemberParams{
			ConversationID: params.ConversationID,
			UserID:         userID,
		})
		if err == nil {
			err = apiConfig.DB.SetMemberRole(r.Context(), database.SetMemberRoleParams{
				ConversationID: params.ConversationID,
				UserID:         next.UserID,
				Role:           database.MemberRoleSuperAdmin,
			})
			if err != nil {
				respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to assign new super admin"})
				return
			}
		}
	}

	if err := apiConfig.Cache.Delete(r.Context(), cache.KeyConversationMembers(params.ConversationID.String())); err != nil {
		log.Printf("Failed to invalidate conversation members cache: %v", err)
	}

	respondWithJSON(w, 200, model.APIResponse{
		Success: true,
		Message: "Member removed successfully",
	})
}

func (apiConfig *apiConfig) handlerSetRole(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(contextKeyUserID).(uuid.UUID)
	if !ok {
		respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Unauthorized"})
		return
	}

	type parameters struct {
		ConversationID uuid.UUID           `json:"conversation_id"`
		UserID         uuid.UUID           `json:"user_id"`
		Role           database.MemberRole `json:"role"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Invalid request payload"})
		return
	}

	// cannot assign super_admin directly, must use transfer endpoint
	if params.Role == database.MemberRoleSuperAdmin {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Use transfer-ownership endpoint to assign super admin"})
		return
	}

	requester, err := apiConfig.DB.GetConversationMember(r.Context(), database.GetConversationMemberParams{
		ConversationID: params.ConversationID,
		UserID:         userID,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			respondWithJSON(w, 403, model.APIResponse{Success: false, Message: "Not a member of this conversation"})
			return
		}
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to verify membership"})
		return
	}

	if requester.Role != database.MemberRoleSuperAdmin {
		respondWithJSON(w, 403, model.APIResponse{Success: false, Message: "Only super admin can assign roles"})
		return
	}

	err = apiConfig.DB.SetMemberRole(r.Context(), database.SetMemberRoleParams{
		ConversationID: params.ConversationID,
		UserID:         params.UserID,
		Role:           params.Role,
	})
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to update role"})
		return
	}

	if err := apiConfig.Cache.Delete(r.Context(), cache.KeyConversationMembers(params.ConversationID.String())); err != nil {
		log.Printf("Failed to invalidate conversation members cache: %v", err)
	}

	respondWithJSON(w, 200, model.APIResponse{
		Success: true,
		Message: "Role updated successfully",
	})
}

func (apiConfig *apiConfig) handlerTransferOwnership(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(contextKeyUserID).(uuid.UUID)
	if !ok {
		respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Unauthorized"})
		return
	}

	type parameters struct {
		ConversationID uuid.UUID `json:"conversation_id"`
		UserID         uuid.UUID `json:"user_id"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Invalid request payload"})
		return
	}

	requester, err := apiConfig.DB.GetConversationMember(r.Context(), database.GetConversationMemberParams{
		ConversationID: params.ConversationID,
		UserID:         userID,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			respondWithJSON(w, 403, model.APIResponse{Success: false, Message: "Not a member of this conversation"})
			return
		}
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to verify membership"})
		return
	}

	if requester.Role != database.MemberRoleSuperAdmin {
		respondWithJSON(w, 403, model.APIResponse{Success: false, Message: "Only super admin can transfer ownership"})
		return
	}

	// demote current super admin to admin
	err = apiConfig.DB.SetMemberRole(r.Context(), database.SetMemberRoleParams{
		ConversationID: params.ConversationID,
		UserID:         userID,
		Role:           database.MemberRoleAdmin,
	})
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to demote super admin"})
		return
	}

	// promote new super admin
	err = apiConfig.DB.SetMemberRole(r.Context(), database.SetMemberRoleParams{
		ConversationID: params.ConversationID,
		UserID:         params.UserID,
		Role:           database.MemberRoleSuperAdmin,
	})
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to promote new super admin"})
		return
	}
	if err := apiConfig.Cache.Delete(r.Context(), cache.KeyConversationMembers(params.ConversationID.String())); err != nil {
		log.Printf("Failed to invalidate conversation members cache: %v", err)
	}
	respondWithJSON(w, 200, model.APIResponse{
		Success: true,
		Message: "Ownership transferred successfully",
	})
}

func (apiConfig *apiConfig) handlerRenameGroup(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(contextKeyUserID).(uuid.UUID)
	if !ok {
		respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Unauthorized"})
		return
	}

	type parameters struct {
		ConversationID uuid.UUID `json:"conversation_id"`
		Name           string    `json:"name"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Invalid request payload"})
		return
	}

	if params.Name == "" {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Name required"})
		return
	}

	member, err := apiConfig.DB.GetConversationMember(r.Context(), database.GetConversationMemberParams{
		ConversationID: params.ConversationID,
		UserID:         userID,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			respondWithJSON(w, 403, model.APIResponse{Success: false, Message: "Not a member of this conversation"})
			return
		}
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to verify membership"})
		return
	}

	if member.Role == database.MemberRoleMember {
		respondWithJSON(w, 403, model.APIResponse{Success: false, Message: "Only admins can rename the group"})
		return
	}

	conversation, err := apiConfig.DB.UpdateConversationName(r.Context(), database.UpdateConversationNameParams{
		ID:   params.ConversationID,
		Name: sql.NullString{String: params.Name, Valid: true},
	})
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to rename group"})
		return
	}
	if err := apiConfig.Cache.DeleteByPattern(r.Context(), fmt.Sprintf("conversations:list:%s:*", userID.String())); err != nil {
		log.Printf("Failed to invalidate conversations cache: %v", err)
	}
	respondWithJSON(w, 200, model.APIResponse{
		Success: true,
		Message: "Group renamed successfully",
		Data:    conversation,
	})
}

func (apiConfig *apiConfig) handlerGetConversations(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(contextKeyUserID).(uuid.UUID)
	if !ok {
		respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Unauthorized"})
		return
	}

	type parameters struct {
		Limit int32 `json:"limit"`
		Page  int32 `json:"page"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Invalid request payload"})
		return
	}

	if params.Limit == 0 {
		params.Limit = 20
	}
	if params.Page == 0 {
		params.Page = 1
	}

	cacheKey := cache.KeyConversationsList(userID.String(), params.Page, params.Limit)

	// try cache first
	if cached, err := apiConfig.Cache.Get(r.Context(), cacheKey); err == nil {
		var conversations interface{}
		if err := json.Unmarshal([]byte(cached), &conversations); err == nil {
			respondWithJSON(w, 200, model.APIResponse{
				Success: true,
				Message: "Conversations fetched successfully",
				Data: map[string]interface{}{
					"conversations": conversations,
					"limit":         params.Limit,
					"page":          params.Page,
				},
			})
			return
		}
	}

	offset := (params.Page - 1) * params.Limit

	conversations, err := apiConfig.DB.GetUserConversations(r.Context(), database.GetUserConversationsParams{
		UserID: userID,
		Limit:  params.Limit,
		Offset: offset,
	})
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to fetch conversations"})
		return
	}

	// store in cache
	if data, err := json.Marshal(conversations); err == nil {
		if err := apiConfig.Cache.Set(r.Context(), cacheKey, string(data), cache.TTLConversationsList); err != nil {
			log.Printf("Failed to cache conversations list: %v", err)
		}
	}

	respondWithJSON(w, 200, model.APIResponse{
		Success: true,
		Message: "Conversations fetched successfully",
		Data: map[string]interface{}{
			"conversations": conversations,
			"limit":         params.Limit,
			"page":          params.Page,
		},
	})
}

func (apiConfig *apiConfig) handlerGetConversationMembers(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(contextKeyUserID).(uuid.UUID)
	if !ok {
		respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Unauthorized"})
		return
	}

	type parameters struct {
		ConversationID uuid.UUID `json:"conversation_id"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Invalid request payload"})
		return
	}

	_, err := apiConfig.DB.GetConversationMember(r.Context(), database.GetConversationMemberParams{
		ConversationID: params.ConversationID,
		UserID:         userID,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			respondWithJSON(w, 403, model.APIResponse{Success: false, Message: "Not a member of this conversation"})
			return
		}
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to verify membership"})
		return
	}

	cacheKey := cache.KeyConversationMembers(params.ConversationID.String())

	// try cache first
	if cached, err := apiConfig.Cache.Get(r.Context(), cacheKey); err == nil {
		var members interface{}
		if err := json.Unmarshal([]byte(cached), &members); err == nil {
			respondWithJSON(w, 200, model.APIResponse{
				Success: true,
				Message: "Members fetched successfully",
				Data:    members,
			})
			return
		}
	}

	members, err := apiConfig.DB.GetConversationMembers(r.Context(), params.ConversationID)
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to fetch members"})
		return
	}

	// store in cache
	if data, err := json.Marshal(members); err == nil {
		if err := apiConfig.Cache.Set(r.Context(), cacheKey, string(data), cache.TTLConversationMembers); err != nil {
			log.Printf("Failed to cache conversation members: %v", err)
		}
	}

	respondWithJSON(w, 200, model.APIResponse{
		Success: true,
		Message: "Members fetched successfully",
		Data:    members,
	})
}

func (apiConfig *apiConfig) handlerGetMessages(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(contextKeyUserID).(uuid.UUID)
	if !ok {
		respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Unauthorized"})
		return
	}

	type parameters struct {
		ConversationID uuid.UUID `json:"conversation_id"`
		Limit          int32     `json:"limit"`
		Page           int32     `json:"page"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Invalid request payload"})
		return
	}

	if params.ConversationID == uuid.Nil {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "conversation_id required"})
		return
	}

	if params.Limit == 0 {
		params.Limit = 50
	}
	if params.Page == 0 {
		params.Page = 1
	}

	offset := (params.Page - 1) * params.Limit

	_, err := apiConfig.DB.GetConversationMember(r.Context(), database.GetConversationMemberParams{
		ConversationID: params.ConversationID,
		UserID:         userID,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			respondWithJSON(w, 403, model.APIResponse{Success: false, Message: "Not a member of this conversation"})
			return
		}
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to verify membership"})
		return
	}

	messages, err := apiConfig.DB.GetMessagesByConversation(r.Context(), database.GetMessagesByConversationParams{
		ConversationID: params.ConversationID,
		Limit:          params.Limit,
		Offset:         offset,
	})
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to fetch messages"})
		return
	}

	err = apiConfig.DB.UpdateLastRead(r.Context(), database.UpdateLastReadParams{
		ConversationID: params.ConversationID,
		UserID:         userID,
	})
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to update last read"})
		return
	}

	respondWithJSON(w, 200, model.APIResponse{
		Success: true,
		Message: "Messages fetched successfully",
		Data: map[string]interface{}{
			"messages": messages,
			"limit":    params.Limit,
			"page":     params.Page,
		},
	})
}

func (apiConfig *apiConfig) handlerSearchMessages(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(contextKeyUserID).(uuid.UUID)
	if !ok {
		respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Unauthorized"})
		return
	}

	type parameters struct {
		ConversationID uuid.UUID `json:"conversation_id"`
		Query          string    `json:"query"`
		Limit          int32     `json:"limit"`
		Page           int32     `json:"page"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Invalid request payload"})
		return
	}

	if params.ConversationID == uuid.Nil {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "conversation_id required"})
		return
	}

	if params.Query == "" {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "query required"})
		return
	}

	if params.Limit == 0 {
		params.Limit = 20
	}
	if params.Page == 0 {
		params.Page = 1
	}

	offset := (params.Page - 1) * params.Limit

	// verify membership
	_, err := apiConfig.DB.GetConversationMember(r.Context(), database.GetConversationMemberParams{
		ConversationID: params.ConversationID,
		UserID:         userID,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			respondWithJSON(w, 403, model.APIResponse{Success: false, Message: "Not a member of this conversation"})
			return
		}
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to verify membership"})
		return
	}

	messages, err := apiConfig.DB.SearchMessages(r.Context(), database.SearchMessagesParams{
		ConversationID: params.ConversationID,
		Column2:        sql.NullString{String: params.Query, Valid: true},
		Limit:          params.Limit,
		Offset:         offset,
	})
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to search messages"})
		return
	}

	respondWithJSON(w, 200, model.APIResponse{
		Success: true,
		Message: "Search results fetched successfully",
		Data: map[string]interface{}{
			"messages": messages,
			"query":    params.Query,
			"limit":    params.Limit,
			"page":     params.Page,
		},
	})
}

func (apiConfig *apiConfig) handlerGetOnlineMembers(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(contextKeyUserID).(uuid.UUID)
	if !ok {
		respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Unauthorized"})
		return
	}

	type parameters struct {
		ConversationID uuid.UUID `json:"conversation_id"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Invalid request payload"})
		return
	}

	// verify membership
	_, err := apiConfig.DB.GetConversationMember(r.Context(), database.GetConversationMemberParams{
		ConversationID: params.ConversationID,
		UserID:         userID,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			respondWithJSON(w, 403, model.APIResponse{Success: false, Message: "Not a member of this conversation"})
			return
		}
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to verify membership"})
		return
	}

	onlineUsers := apiConfig.Hub.GetOnlineUsers(params.ConversationID)

	respondWithJSON(w, 200, model.APIResponse{
		Success: true,
		Message: "Online members fetched successfully",
		Data: map[string]interface{}{
			"online_users": onlineUsers,
		},
	})
}

func (apiConfig *apiConfig) handlerDeleteConversation(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(contextKeyUserID).(uuid.UUID)
	if !ok {
		respondWithJSON(w, 401, model.APIResponse{Success: false, Message: "Unauthorized"})
		return
	}

	type parameters struct {
		ConversationID uuid.UUID `json:"conversation_id"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	if err := decoder.Decode(&params); err != nil {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "Invalid request payload"})
		return
	}

	if params.ConversationID == uuid.Nil {
		respondWithJSON(w, 400, model.APIResponse{Success: false, Message: "conversation_id required"})
		return
	}

	conversation, err := apiConfig.DB.GetConversationByID(r.Context(), params.ConversationID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondWithJSON(w, 404, model.APIResponse{Success: false, Message: "Conversation not found"})
			return
		}
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to fetch conversation"})
		return
	}

	member, err := apiConfig.DB.GetConversationMember(r.Context(), database.GetConversationMemberParams{
		ConversationID: params.ConversationID,
		UserID:         userID,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			respondWithJSON(w, 403, model.APIResponse{Success: false, Message: "Not a member of this conversation"})
			return
		}
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to verify membership"})
		return
	}

	// group: only super_admin can delete
	if conversation.IsGroup && member.Role != database.MemberRoleSuperAdmin {
		respondWithJSON(w, 403, model.APIResponse{Success: false, Message: "Only super admin can delete the group"})
		return
	}

	err = apiConfig.DB.DeleteConversation(r.Context(), params.ConversationID)
	if err != nil {
		respondWithJSON(w, 500, model.APIResponse{Success: false, Message: "Failed to delete conversation"})
		return
	}

	// notify all online members that conversation was deleted
	apiConfig.Hub.BroadcastToConversation(params.ConversationID, userID, ws.OutgoingMessage{
		Type:           ws.TypeDelete,
		ConversationID: &params.ConversationID,
		SenderID:       &userID,
	})
	if err := apiConfig.Cache.DeleteByPattern(r.Context(), fmt.Sprintf("conversations:list:%s:*", userID.String())); err != nil {
		log.Printf("Failed to invalidate conversations cache: %v", err)
	}
	respondWithJSON(w, 200, model.APIResponse{
		Success: true,
		Message: "Conversation deleted successfully",
	})
}
