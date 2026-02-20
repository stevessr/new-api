package controller

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type SubscriptionWalletPayRequest struct {
	PlanId int `json:"plan_id"`
}

func SubscriptionRequestWalletPay(c *gin.Context) {
	var req SubscriptionWalletPayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	userId := c.GetInt("id")
	tradeNo, err := model.PurchaseSubscriptionWithWallet(userId, req.PlanId)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrSubscriptionPlanDisabled):
			common.ApiErrorMsg(c, "套餐未启用")
			return
		case errors.Is(err, model.ErrSubscriptionWalletInsufficient):
			common.ApiErrorMsg(c, "余额不足")
			return
		case errors.Is(err, model.ErrSubscriptionPurchaseLimitExceeded):
			common.ApiErrorMsg(c, err.Error())
			return
		default:
			common.ApiError(c, err)
			return
		}
	}

	common.ApiSuccess(c, gin.H{"trade_no": tradeNo})
}
