package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/ldc"
	"github.com/QuantumNous/new-api/setting"
)

func requestLDCSubscription(c *gin.Context, plan *model.SubscriptionPlan) {
	if !isLDCTopUpEnabled() {
		common.ApiErrorMsg(c, "当前管理员未配置 LDC 支付信息")
		return
	}
	client, err := newLDCClient()
	if err != nil {
		common.ApiErrorMsg(c, "当前管理员未配置 LDC 支付信息")
		return
	}
	userID := c.GetInt("id")
	tradeNo := fmt.Sprintf("SUBUSR%dLDC%d%s", userID, time.Now().Unix(), common.GetRandomString(6))
	returnURL := strings.TrimSpace(setting.LDCReturnURL)
	if returnURL == "" {
		returnURL = paymentReturnPath("/wallet?pay=pending")
	}
	returnAddress, err := parseLDCURL(returnURL)
	if err != nil {
		common.ApiErrorMsg(c, "回跳地址配置错误")
		return
	}
	notifyAddress, err := parseLDCURL(ldcNotifyURL())
	if err != nil {
		common.ApiErrorMsg(c, "回调地址配置错误")
		return
	}

	order := &model.SubscriptionOrder{
		UserId:          userID,
		PlanId:          plan.Id,
		Money:           plan.PriceAmount,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodLDC,
		PaymentProvider: model.PaymentProviderLDC,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := order.Insert(); err != nil {
		common.ApiErrorMsg(c, "创建订单失败")
		return
	}
	checkoutURL, err := client.CreateOrder(c.Request.Context(), ldc.CreateOrderRequest{
		ServiceTradeNo: tradeNo,
		OrderName:      fmt.Sprintf("SUB:%s", plan.Title),
		Amount:         strconv.FormatFloat(plan.PriceAmount, 'f', 2, 64),
		NotifyURL:      notifyAddress.String(),
		ReturnURL:      returnAddress.String(),
	})
	if err != nil {
		_ = model.ExpireSubscriptionOrder(tradeNo, model.PaymentProviderLDC)
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": gin.H{}, "url": checkoutURL})
}
