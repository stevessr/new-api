package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createLDCSettlementTestOrder(t *testing.T, userID int, tradeNo string, provider string) *TopUp {
	t.Helper()
	order := &TopUp{
		UserId: userID,
		Amount: 2,
		Money: 9.99,
		TradeNo: tradeNo,
		PaymentMethod: PaymentMethodLDC,
		PaymentProvider: provider,
		CreateTime: common.GetTimestamp(),
		Status: common.TopUpStatusPending,
	}
	require.NoError(t, order.Insert())
	return order
}

func TestRechargeLDCCreditsExactlyOnce(t *testing.T) {
	truncateTables(t)
	original := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = original })

	user := insertUserForPaymentGuardTest(t, 621, 7)
	order := createLDCSettlementTestOrder(t, user.Id, "LDCTESTONCE", PaymentProviderLDC)

	alreadyDone, err := RechargeLDC(order.TradeNo, "9.99", "127.0.0.1")
	require.NoError(t, err)
	assert.False(t, alreadyDone)
	assert.Equal(t, 1000007, getUserQuotaForPaymentGuardTest(t, user.Id))

	settled := GetTopUpByTradeNo(order.TradeNo)
	require.NotNil(t, settled)
	assert.Equal(t, common.TopUpStatusSuccess, settled.Status)
	assert.NotZero(t, settled.CompleteTime)

	alreadyDone, err = RechargeLDC(order.TradeNo, "9.99", "127.0.0.1")
	require.NoError(t, err)
	assert.True(t, alreadyDone)
	assert.Equal(t, 1000007, getUserQuotaForPaymentGuardTest(t, user.Id))
}

func TestRechargeLDCRejectsTamperedAmountOrWrongProvider(t *testing.T) {
	truncateTables(t)
	original := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = original })

	user := insertUserForPaymentGuardTest(t, 622, 0)
	order := createLDCSettlementTestOrder(t, user.Id, "LDCTESTBADAMOUNT", PaymentProviderLDC)
	_, err := RechargeLDC(order.TradeNo, "99.99", "127.0.0.1")
	require.Error(t, err)
	assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, order.TradeNo))
	assert.Zero(t, getUserQuotaForPaymentGuardTest(t, user.Id))

	foreign := createLDCSettlementTestOrder(t, user.Id, "LDCTESTFOREIGN", PaymentProviderStripe)
	_, err = RechargeLDC(foreign.TradeNo, "9.99", "127.0.0.1")
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)
	assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, foreign.TradeNo))
	assert.Zero(t, getUserQuotaForPaymentGuardTest(t, user.Id))
}

func TestRechargeLDCRollsBackWhenWalletIsFull(t *testing.T) {
	truncateTables(t)
	original := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = original })

	initialQuota := common.MaxWalletQuota - 1
	user := insertUserForPaymentGuardTest(t, 623, initialQuota)
	order := createLDCSettlementTestOrder(t, user.Id, "LDCTESTROLLBACK", PaymentProviderLDC)

	alreadyDone, err := RechargeLDC(order.TradeNo, "9.99", "127.0.0.1")
	require.ErrorIs(t, err, ErrTopUpQuotaLimitExceeded)
	assert.False(t, alreadyDone)
	assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, order.TradeNo))
	assert.Equal(t, initialQuota, getUserQuotaForPaymentGuardTest(t, user.Id))
}

func TestRechargeLDCUpdatesCachedQuota(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	original := common.QuotaPerUnit
	common.QuotaPerUnit = 5
	t.Cleanup(func() { common.QuotaPerUnit = original })

	user := insertUserForPaymentGuardTest(t, 624, 7)
	require.NoError(t, populateUserCache(*user))
	order := createLDCSettlementTestOrder(t, user.Id, "LDCTESTCACHE", PaymentProviderLDC)

	alreadyDone, err := RechargeLDC(order.TradeNo, "9.99", "127.0.0.1")
	require.NoError(t, err)
	assert.False(t, alreadyDone)
	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 17, cached.Quota)
	assert.Equal(t, 17, getUserQuotaForPaymentGuardTest(t, user.Id))
}
