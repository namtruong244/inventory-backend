package handler

import (
	"net/http"

	"inventory_backend/internal/middleware"
	"inventory_backend/internal/service"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authService service.AuthService
}

func NewAuthHandler(authService service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

type RegisterRequest struct {
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type GoogleLoginRequest struct {
	Email            string  `json:"email" binding:"required,email"`
	Name             string  `json:"name" binding:"required"`
	AvatarURL        *string `json:"avatar_url"`
	AvatarUrl        *string `json:"avatarUrl"`
	IDToken          *string `json:"id_token"`
	IdToken          *string `json:"idToken"`
	AccessToken      *string `json:"access_token"`
	AccessTokenCamel *string `json:"accessToken"`
}

type AppleLoginRequest struct {
	Email                  string  `json:"email" binding:"required,email"`
	Name                   string  `json:"name" binding:"required"`
	IdentityToken          *string `json:"identity_token"`
	IdentityTokenCamel     *string `json:"identityToken"`
	AuthorizationCode      *string `json:"authorization_code"`
	AuthorizationCodeCamel *string `json:"authorizationCode"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_INPUT", err.Error(), nil)
		return
	}

	result, err := h.authService.Register(req.Name, req.Email, req.Password)
	if err != nil {
		SendError(c, http.StatusBadRequest, "REGISTRATION_FAILED", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusCreated, result, "Account registered successfully")
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_INPUT", err.Error(), nil)
		return
	}

	result, err := h.authService.Login(req.Email, req.Password)
	if err != nil {
		SendError(c, http.StatusUnauthorized, "AUTH_FAILED", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusOK, result, "Login successful")
}

func (h *AuthHandler) GoogleLogin(c *gin.Context) {
	var req GoogleLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_INPUT", err.Error(), nil)
		return
	}

	avatarURL := req.AvatarURL
	if avatarURL == nil && req.AvatarUrl != nil {
		avatarURL = req.AvatarUrl
	}

	idToken := req.IDToken
	if idToken == nil && req.IdToken != nil {
		idToken = req.IdToken
	}

	result, err := h.authService.LoginWithGoogle(req.Email, req.Name, avatarURL, idToken)
	if err != nil {
		SendError(c, http.StatusBadRequest, "AUTH_FAILED", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusOK, result, "Google authentication successful")
}

func (h *AuthHandler) AppleLogin(c *gin.Context) {
	var req AppleLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, http.StatusBadRequest, "INVALID_INPUT", err.Error(), nil)
		return
	}

	identityToken := req.IdentityToken
	if identityToken == nil && req.IdentityTokenCamel != nil {
		identityToken = req.IdentityTokenCamel
	}

	result, err := h.authService.LoginWithApple(req.Email, req.Name, identityToken)
	if err != nil {
		SendError(c, http.StatusBadRequest, "AUTH_FAILED", err.Error(), nil)
		return
	}

	SendSuccess(c, http.StatusOK, result, "Apple authentication successful")
}

func (h *AuthHandler) Me(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		SendError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated", nil)
		return
	}

	user, err := h.authService.GetProfile(userID)
	if err != nil {
		SendError(c, http.StatusNotFound, "USER_NOT_FOUND", "User profile not found", nil)
		return
	}

	SendSuccess(c, http.StatusOK, service.UserResponse{
		ID:        user.ID,
		Name:      user.Name,
		Email:     user.Email,
		Role:      user.Role,
		AvatarURL: user.AvatarURL,
		Provider:  user.Provider,
	}, "")
}
