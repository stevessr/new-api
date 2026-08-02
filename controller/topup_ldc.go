package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/ldc"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

const maxLDCQuota int64 = 1<<31 - 1

func maxLDCRequestAmount() int64 {
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		return maxLDCQuota
	}
	maxAmount := decimal.NewFromInt(maxLDCQuota).Div(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart()
	if maxAmount < 1 {
		return 1
	}
	return maxAmount
}

func newLDCClient() (*ldc.Client, error) {
	return ldc.NewClient(ldc.Config{
		BaseURL:      setting.LDCBaseURL,
		ClientID:     setting.LDCClientID,
		ClientSecret: setting.LDCClientSecret,
		PrivateKey:   setting.LDCPrivateKey,
	})
}

func getLDCMinTopup() int64 {
	minTopup := setting.LDCMinTopUp
	if minTopup <= 0 {
		return 0
	}
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		quotaMin := decimal.NewFromInt(int64(minTopup)).Mul(decimal.NewFromFloat(common.QuotaPerUnit))
		if quotaMin.GreaterThan(decimal.NewFromInt(maxLDCQuota)) {
			return maxLDCQuota
		}
		return quotaMin.IntPart()
	}
	return int64(minTopup)
}

func ldcNotifyURL() string {
	if notifyURL := strings.TrimSpace(setting.LDCNotifyURL); notifyURL != "" {
		return notifyURL
	}
	return strings.TrimRight(service.GetCallbackAddress(), "/") + "/api/ldc/notify"
}

func ldcReturnURL() string {
	if returnURL := strings.TrimSpace(setting.LDCReturnURL); returnURL != "" {
		return returnURL
	}
	return paymentReturnPath("/usage-logs")
}

func parseLDCURL(value string) (*url.URL, error) {
	parsedURL, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, errors.New("invalid LDC URL")
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, errors.New("LDC URL must use http or https")
	}
	return parsedURL, nil
}

func requestLDCOrder(c *gin.Context, req EpayRequest) {
	if !isLDCTopUpEnabled() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "当前管理员未配置 LDC 支付信息"})
		return
	}
	minTopup := getLDCMinTopup()
	if req.Amount < minTopup {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", minTopup)})
		return
	}
	if req.Amount > maxLDCRequestAmount() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值数量过大"})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getPayMoney(req.Amount, group)
	if payMoney < 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	client, err := newLDCClient()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("LDC 初始化支付客户端失败 user_id=%d error=%q", id, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "当前管理员未配置支付信息"})
		return
	}
	tradeNo := fmt.Sprintf("USR%dLD%d%s", id, time.Now().Unix(), common.GetRandomString(6))
	returnURL, err := parseLDCURL(ldcReturnURL())
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "回跳地址配置错误"})
		return
	}
	notifyURL, err := parseLDCURL(ldcNotifyURL())
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "回调地址配置错误"})
		return
	}
	checkoutURL, err := client.CreateOrder(c.Request.Context(), ldc.CreateOrderRequest{
		ServiceTradeNo: tradeNo,
		OrderName:      fmt.Sprintf("TUC%d", req.Amount),
		Amount:         strconv.FormatFloat(payMoney, 'f', 2, 64),
		NotifyURL:      notifyURL.String(),
		ReturnURL:      returnURL.String(),
	})
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("LDC 拉起支付失败 user_id=%d trade_no=%s amount=%d error=%q", id, tradeNo, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}

	amount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		amount = decimal.NewFromInt(amount).Div(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart()
	}
	topUp := &model.TopUp{
		UserId:          id,
		Amount:          amount,
		Money:           payMoney,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodLDC,
		PaymentProvider: model.PaymentProviderLDC,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("LDC 创建充值订单失败 user_id=%d trade_no=%s amount=%d error=%q", id, tradeNo, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("LDC 充值订单创建成功 user_id=%d trade_no=%s amount=%d money=%.2f url=%q", id, tradeNo, req.Amount, payMoney, checkoutURL))
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": gin.H{}, "url": checkoutURL})
}

func RequestLDC(c *gin.Context) {
	var req EpayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if req.PaymentMethod != model.PaymentMethodLDC {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付方式不存在"})
		return
	}
	requestLDCOrder(c, req)
}

func LDCNotify(c *gin.Context) {
	if !isLDCWebhookEnabled() {
		c.String(http.StatusOK, "fail")
		return
	}
	if err := c.Request.ParseForm(); err != nil {
		c.String(http.StatusOK, "fail")
		return
	}
	client, err := newLDCClient()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("LDC 回调初始化支付客户端失败 error=%q", err.Error()))
		c.String(http.StatusOK, "fail")
		return
	}
	notification, err := client.VerifyNotification(c.Request.Form)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("LDC 回调验签失败 error=%q", err.Error()))
		c.String(http.StatusOK, "fail")
		return
	}
	if notification.TradeStatus != ldc.TradeStatusOK {
		c.String(http.StatusOK, "success")
		return
	}
	if err := completeLDCOrder(c, notification, c.Request.Form); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("LDC 回调结算失败 trade_no=%s error=%q", notification.ServiceTradeNo, err.Error()))
		c.String(http.StatusOK, "fail")
		return
	}
	c.String(http.StatusOK, "success")
}

func completeLDCOrder(c *gin.Context, notification ldc.Notification, params url.Values) error {
	if notification.ServiceTradeNo == "" {
		return errors.New("LDC service trade number is empty")
	}
	LockOrder(notification.ServiceTradeNo)
	defer UnlockOrder(notification.ServiceTradeNo)

	if subscriptionOrder := model.GetSubscriptionOrderByTradeNo(notification.ServiceTradeNo); subscriptionOrder != nil {
		if subscriptionOrder.PaymentProvider != model.PaymentProviderLDC {
			return model.ErrPaymentMethodMismatch
		}
		if !ldcMoneyMatches(subscriptionOrder.Money, notification.Money) {
			return errors.New("LDC subscription payment amount mismatch")
		}
		return model.CompleteSubscriptionOrder(
			notification.ServiceTradeNo,
			common.GetJsonString(params),
			model.PaymentProviderLDC,
			model.PaymentMethodLDC,
		)
	}

	topUp := model.GetTopUpByTradeNo(notification.ServiceTradeNo)
	if topUp == nil {
		return model.ErrTopUpNotFound
	}
	if topUp.PaymentProvider != model.PaymentProviderLDC {
		return model.ErrPaymentMethodMismatch
	}
	if !ldcMoneyMatches(topUp.Money, notification.Money) {
		return errors.New("LDC top-up payment amount mismatch")
	}
	if topUp.Status == common.TopUpStatusSuccess {
		return nil
	}
	if topUp.Status != common.TopUpStatusPending {
		return model.ErrTopUpStatusInvalid
	}

	quota, clamp := common.QuotaFromDecimalChecked(decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
	if clamp != nil {
		return errors.New("LDC top-up quota is out of range")
	}
	if quota <= 0 {
		return errors.New("LDC top-up quota is invalid")
	}
	if err := model.UpdatePendingTopUpStatus(topUp.TradeNo, model.PaymentProviderLDC, common.TopUpStatusSuccess); err != nil {
		if errors.Is(err, model.ErrTopUpStatusInvalid) {
			if current := model.GetTopUpByTradeNo(notification.ServiceTradeNo); current != nil && current.Status == common.TopUpStatusSuccess {
				return nil
			}
		}
		return err
	}
	if err := model.IncreaseUserQuota(topUp.UserId, quota, true); err != nil {
		return err
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("LDC 充值成功 user_id=%d trade_no=%s quota=%d money=%.2f", topUp.UserId, topUp.TradeNo, quota, topUp.Money))
	model.RecordTopupLog(
		topUp.UserId,
		fmt.Sprintf("使用 LDC 充值成功，充值金额: %v，支付金额：%f", logger.LogQuota(quota), topUp.Money),
		c.ClientIP(),
		topUp.PaymentMethod,
		model.PaymentProviderLDC,
	)
	return nil
}

func ldcMoneyMatches(expected float64, actual string) bool {
	actualMoney, err := decimal.NewFromString(strings.TrimSpace(actual))
	if err != nil {
		return false
	}
	return decimal.NewFromFloat(expected).Round(2).Equal(actualMoney.Round(2))
}
