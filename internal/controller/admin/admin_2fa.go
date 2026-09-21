package admin

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/middleware"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/pkg/response"
)

type totpCodeReq struct {
	Code string `json:"code"`
}

type recoveryAckReq struct {
	Confirmed bool `json:"confirmed"`
}

type verify2FAReq struct {
	TOTP         string `json:"totp"`
	RecoveryCode string `json:"recovery_code"`
}

type webauthnFinishReq struct {
	Name       string          `json:"name"`
	Credential json.RawMessage `json:"credential"`
}

func contextToken(c *gin.Context) string {
	v, _ := c.Get(middleware.ContextAdminToken)
	tok, _ := v.(string)
	return tok
}

func contextAdminID(c *gin.Context) (uuid.UUID, bool) {
	v, _ := c.Get(middleware.ContextAdminID)
	s, _ := v.(string)
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

func (h *handler) logout(c *gin.Context) {
	if err := h.admins.Logout(c.Request.Context(), contextToken(c)); err != nil {
		writeAdminErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *handler) totpSetup(c *gin.Context) {
	out, err := h.admins.SetupTOTP(c.Request.Context(), contextToken(c))
	if err != nil {
		writeAdminErr(c, err)
		return
	}
	writeLoginResult(c, out)
}

func (h *handler) totpConfirm(c *gin.Context) {
	var req totpCodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	out, err := h.admins.ConfirmTOTP(c.Request.Context(), contextToken(c), req.Code)
	if err != nil {
		writeAdminErr(c, err)
		return
	}
	writeLoginResult(c, out)
}

func (h *handler) recoveryAck(c *gin.Context) {
	var req recoveryAckReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	out, err := h.admins.AckRecovery(c.Request.Context(), contextToken(c), req.Confirmed)
	if err != nil {
		writeAdminErr(c, err)
		return
	}
	writeLoginResult(c, out)
}

func (h *handler) skipPasskey(c *gin.Context) {
	out, err := h.admins.SkipPasskey(c.Request.Context(), contextToken(c))
	if err != nil {
		writeAdminErr(c, err)
		return
	}
	writeLoginResult(c, out)
}

func (h *handler) verify2FA(c *gin.Context) {
	var req verify2FAReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	var (
		out *service.LoginResult
		err error
	)
	switch {
	case req.RecoveryCode != "":
		out, err = h.admins.VerifyRecovery(c.Request.Context(), contextToken(c), req.RecoveryCode)
	case req.TOTP != "":
		out, err = h.admins.VerifyTOTP(c.Request.Context(), contextToken(c), req.TOTP)
	default:
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "totp or recovery_code required", nil)
		return
	}
	if err != nil {
		writeAdminErr(c, err)
		return
	}
	writeLoginResult(c, out)
}

func (h *handler) webauthnLoginBegin(c *gin.Context) {
	raw, err := h.admins.BeginPasskeyLogin(c.Request.Context(), contextToken(c))
	if err != nil {
		writeAdminErr(c, err)
		return
	}
	writeWebAuthnOptions(c, raw)
}

func (h *handler) webauthnLoginFinish(c *gin.Context) {
	var req webauthnFinishReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	out, err := h.admins.FinishPasskeyLogin(c.Request.Context(), contextToken(c), req.Credential)
	if err != nil {
		writeAdminErr(c, err)
		return
	}
	writeLoginResult(c, out)
}

func (h *handler) webauthnRegisterBeginPending(c *gin.Context) {
	id, ok := contextAdminID(c)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token", nil)
		return
	}
	raw, err := h.admins.BeginPasskeyRegister(c.Request.Context(), id, contextToken(c))
	if err != nil {
		writeAdminErr(c, err)
		return
	}
	writeWebAuthnOptions(c, raw)
}

func (h *handler) webauthnRegisterFinishPending(c *gin.Context) {
	id, ok := contextAdminID(c)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token", nil)
		return
	}
	var req webauthnFinishReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	out, err := h.admins.FinishPasskeyRegister(c.Request.Context(), id, contextToken(c), req.Name, req.Credential)
	if err != nil {
		writeAdminErr(c, err)
		return
	}
	writeLoginResult(c, out)
}

func (h *handler) twoFAStatus(c *gin.Context) {
	id, ok := contextAdminID(c)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token", nil)
		return
	}
	out, err := h.admins.TwoFAStatus(c.Request.Context(), id)
	if err != nil {
		writeAdminErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{
		"totp_enabled":       out.TOTPEnabled,
		"recovery_remaining": out.RecoveryLeft,
		"passkeys":           publicPasskeys(out.Passkeys),
	})
}

func (h *handler) totpRotateSetup(c *gin.Context) {
	id, ok := contextAdminID(c)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token", nil)
		return
	}
	out, err := h.admins.SetupTOTPRotate(c.Request.Context(), id)
	if err != nil {
		writeAdminErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{
		"secret":      out.Secret,
		"otpauth_url": out.OTPAuthURL,
	})
}

func (h *handler) totpRotateConfirm(c *gin.Context) {
	id, ok := contextAdminID(c)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token", nil)
		return
	}
	var req totpCodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	if err := h.admins.ConfirmTOTPRotate(c.Request.Context(), id, req.Code); err != nil {
		writeAdminErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *handler) recoveryRegenerate(c *gin.Context) {
	id, ok := contextAdminID(c)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token", nil)
		return
	}
	codes, err := h.admins.RegenerateRecovery(c.Request.Context(), id)
	if err != nil {
		writeAdminErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"recovery_codes": codes})
}

func (h *handler) listPasskeys(c *gin.Context) {
	id, ok := contextAdminID(c)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token", nil)
		return
	}
	list, err := h.admins.ListPasskeys(c.Request.Context(), id)
	if err != nil {
		writeAdminErr(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"passkeys": publicPasskeys(list)})
}

func (h *handler) passkeyBegin(c *gin.Context) {
	id, ok := contextAdminID(c)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token", nil)
		return
	}
	raw, err := h.admins.BeginPasskeyRegister(c.Request.Context(), id, "")
	if err != nil {
		writeAdminErr(c, err)
		return
	}
	writeWebAuthnOptions(c, raw)
}

func (h *handler) passkeyFinish(c *gin.Context) {
	id, ok := contextAdminID(c)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token", nil)
		return
	}
	var req webauthnFinishReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid json", nil)
		return
	}
	if _, err := h.admins.FinishPasskeyRegister(c.Request.Context(), id, "", req.Name, req.Credential); err != nil {
		writeAdminErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *handler) deletePasskey(c *gin.Context) {
	adminID, ok := contextAdminID(c)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid token", nil)
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid id", nil)
		return
	}
	if err := h.admins.DeletePasskey(c.Request.Context(), adminID, id); err != nil {
		writeAdminErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func writeWebAuthnOptions(c *gin.Context, raw json.RawMessage) {
	var options any
	if err := json.Unmarshal(raw, &options); err != nil {
		response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"options": options})
}

func publicPasskeys(list []service.PasskeyPublic) []gin.H {
	out := make([]gin.H, 0, len(list))
	for _, p := range list {
		out = append(out, gin.H{
			"id":         p.ID,
			"name":       p.Name,
			"created_at": formatTimeUTC(p.CreatedAt),
		})
	}
	return out
}
