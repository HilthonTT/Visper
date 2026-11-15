package room

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hilthontt/visper/services/api/domain/model"
	"github.com/hilthontt/visper/services/api/infrastructure/websocket"
	"github.com/hilthontt/visper/services/api/presentation/httpx"
)

// Room keys: clients exchange key cards and rekeys through these endpoints
// (see the roomkey use case). Nothing here can be read by the server.

type PutKeyCardRequest struct {
	Card string `json:"card" binding:"required"`
}

type RekeyRequest struct {
	Epoch     int               `json:"epoch" binding:"required,min=1"`
	Keys      map[string]string `json:"keys" binding:"max=1000"`
	Signature string            `json:"signature" binding:"required"`
}

type CryptoStateResponse struct {
	KeyEpoch    int                 `json:"key_epoch"`
	RekeyNeeded bool                `json:"rekey_needed"`
	Rekey       *model.Rekey        `json:"rekey"`
	Cards       map[string][]string `json:"cards"`
	Members     []string            `json:"members"`
}

func (c *roomController) GetCryptoState(ctx *gin.Context) {
	roomID, ok := httpx.RoomID(ctx)
	if !ok {
		return
	}
	user, ok := httpx.RequireUser(ctx)
	if !ok {
		return
	}

	state, err := c.keys.State(ctx.Request.Context(), roomID, user.ID)
	if err != nil {
		httpx.Error(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, CryptoStateResponse{
		KeyEpoch:    state.Epoch,
		RekeyNeeded: state.RekeyNeeded,
		Rekey:       state.Rekey,
		Cards:       state.Cards,
		Members:     state.Members,
	})
}

func (c *roomController) PutKeyCard(ctx *gin.Context) {
	roomID, ok := httpx.RoomID(ctx)
	if !ok {
		return
	}
	var req PutKeyCardRequest
	if !httpx.BindJSON(ctx, &req) {
		return
	}
	user, ok := httpx.RequireUser(ctx)
	if !ok {
		return
	}

	if err := c.keys.PutCard(ctx.Request.Context(), roomID, user.ID, req.Card); err != nil {
		httpx.Error(ctx, err)
		return
	}

	c.wsCore.Publish(websocket.NewMemberCard(roomID, user.ID, req.Card))
	ctx.JSON(http.StatusOK, httpx.SuccessResponse{Message: "key card published"})
}

func (c *roomController) PostRekey(ctx *gin.Context) {
	roomID, ok := httpx.RoomID(ctx)
	if !ok {
		return
	}
	var req RekeyRequest
	if !httpx.BindJSON(ctx, &req) {
		return
	}
	user, ok := httpx.RequireUser(ctx)
	if !ok {
		return
	}

	rekey, err := c.keys.Rekey(ctx.Request.Context(), roomID, user.ID, model.Rekey{
		Epoch:     req.Epoch,
		Keys:      req.Keys,
		Signature: req.Signature,
	})
	if err != nil {
		httpx.Error(ctx, err)
		return
	}

	c.wsCore.Publish(websocket.NewRoomRekeyed(roomID, rekey))
	ctx.JSON(http.StatusOK, httpx.SuccessResponse{Message: "room key changed"})
}

// markDeparted tells the owner's client a rekey is due after userID left.
// The member is out either way, so a failure is only logged.
func (c *roomController) markDeparted(ctx *gin.Context, roomID, userID string) {
	if err := c.keys.MarkDeparted(ctx.Request.Context(), roomID, userID); err != nil {
		_ = ctx.Error(err)
	}
}
