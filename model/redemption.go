package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"gorm.io/gorm"
)

// ErrRedeemFailed is returned when redemption fails due to database error
var ErrRedeemFailed = errors.New("redeem.failed")

const (
	RedemptionTypeQuota        = "quota"
	RedemptionTypeSubscription = "subscription"
)

type RedeemResult struct {
	RedeemType   string              `json:"redeem_type"`
	Quota        int                 `json:"quota"`
	Subscription *RedeemSubscription `json:"subscription,omitempty"`
}

type RedeemSubscription struct {
	UserSubscriptionId int    `json:"user_subscription_id"`
	PlanId             int    `json:"plan_id"`
	PlanTitle          string `json:"plan_title"`
	StartTime          int64  `json:"start_time"`
	EndTime            int64  `json:"end_time"`
}

type Redemption struct {
	Id           int            `json:"id"`
	UserId       int            `json:"user_id"`
	Key          string         `json:"key" gorm:"type:char(32);uniqueIndex"`
	Status       int            `json:"status" gorm:"default:1"`
	Name         string         `json:"name" gorm:"index"`
	RedeemType   string         `json:"redeem_type" gorm:"type:varchar(16);not null;default:'quota';index"`
	Quota        int            `json:"quota" gorm:"default:100"`
	PlanId       int            `json:"plan_id" gorm:"type:int;not null;default:0;index"`
	CreatedTime  int64          `json:"created_time" gorm:"bigint"`
	RedeemedTime int64          `json:"redeemed_time" gorm:"bigint"`
	Count        int            `json:"count" gorm:"-:all"` // only for api request
	UsedUserId   int            `json:"used_user_id"`
	DeletedAt    gorm.DeletedAt `gorm:"index"`
	ExpiredTime  int64          `json:"expired_time" gorm:"bigint"` // 过期时间，0 表示不过期
}

func NormalizeRedemptionType(redeemType string) string {
	switch strings.TrimSpace(redeemType) {
	case RedemptionTypeSubscription:
		return RedemptionTypeSubscription
	case RedemptionTypeQuota:
		return RedemptionTypeQuota
	default:
		return RedemptionTypeQuota
	}
}

func (redemption *Redemption) BeforeCreate(tx *gorm.DB) error {
	redemption.RedeemType = NormalizeRedemptionType(redemption.RedeemType)
	if redemption.RedeemType != RedemptionTypeSubscription {
		redemption.PlanId = 0
	}
	return nil
}

func (redemption *Redemption) BeforeUpdate(tx *gorm.DB) error {
	redemption.RedeemType = NormalizeRedemptionType(redemption.RedeemType)
	if redemption.RedeemType != RedemptionTypeSubscription {
		redemption.PlanId = 0
	}
	return nil
}

func (redemption *Redemption) AfterFind(tx *gorm.DB) error {
	redemption.RedeemType = NormalizeRedemptionType(redemption.RedeemType)
	return nil
}

func redemptionTxWithForUpdate(tx *gorm.DB) *gorm.DB {
	if tx == nil {
		return tx
	}
	if common.UsingSQLite {
		return tx
	}
	return tx.Set("gorm:query_option", "FOR UPDATE")
}

func GetAllRedemptions(startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	// 开始事务
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 获取总数
	err = tx.Model(&Redemption{}).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 获取分页数据
	err = tx.Order("id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 提交事务
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return redemptions, total, nil
}

func SearchRedemptions(keyword string, startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Build query based on keyword type
	query := tx.Model(&Redemption{})

	// Only try to convert to ID if the string represents a valid integer
	if id, err := strconv.Atoi(keyword); err == nil {
		query = query.Where("id = ? OR name LIKE ?", id, keyword+"%")
	} else {
		query = query.Where("name LIKE ?", keyword+"%")
	}

	// Get total count
	err = query.Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated data
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return redemptions, total, nil
}

func GetRedemptionById(id int) (*Redemption, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	redemption := Redemption{Id: id}
	var err error = nil
	err = DB.First(&redemption, "id = ?", id).Error
	return &redemption, err
}

func Redeem(key string, userId int) (result *RedeemResult, err error) {
	if key == "" {
		return nil, errors.New("未提供兑换码")
	}
	if userId == 0 {
		return nil, errors.New("无效的 user id")
	}
	redemption := &Redemption{}
	result = &RedeemResult{
		RedeemType: RedemptionTypeQuota,
		Quota:      0,
	}

	keyCol := "`key`"
	if common.UsingPostgreSQL {
		keyCol = `"key"`
	}
	common.RandomSleep()
	err = DB.Transaction(func(tx *gorm.DB) error {
		err := redemptionTxWithForUpdate(tx).Where(keyCol+" = ?", key).First(redemption).Error
		if err != nil {
			return errors.New("无效的兑换码")
		}
		if redemption.Status != common.RedemptionCodeStatusEnabled {
			return errors.New("该兑换码已被使用")
		}
		if redemption.ExpiredTime != 0 && redemption.ExpiredTime < common.GetTimestamp() {
			return errors.New("该兑换码已过期")
		}
		redeemType := NormalizeRedemptionType(redemption.RedeemType)
		result.RedeemType = redeemType
		switch redeemType {
		case RedemptionTypeSubscription:
			if redemption.PlanId <= 0 {
				return errors.New("兑换码未配置订阅套餐")
			}
			plan, planErr := getSubscriptionPlanByIdTx(tx, redemption.PlanId)
			if planErr != nil || plan == nil {
				return errors.New("订阅套餐不存在")
			}
			sub, subErr := CreateUserSubscriptionFromPlanTx(tx, userId, plan, "redemption")
			if subErr != nil {
				return subErr
			}
			result.Subscription = &RedeemSubscription{
				UserSubscriptionId: sub.Id,
				PlanId:             plan.Id,
				PlanTitle:          plan.Title,
				StartTime:          sub.StartTime,
				EndTime:            sub.EndTime,
			}
			result.Quota = 0
		default:
			if redemption.Quota <= 0 {
				return errors.New("兑换码额度无效")
			}
			err = tx.Model(&User{}).Where("id = ?", userId).Update("quota", gorm.Expr("quota + ?", redemption.Quota)).Error
			if err != nil {
				return err
			}
			result.Quota = redemption.Quota
		}
		redemption.RedeemedTime = common.GetTimestamp()
		redemption.Status = common.RedemptionCodeStatusUsed
		redemption.UsedUserId = userId
		err = tx.Save(redemption).Error
		return err
	})
	if err != nil {
		common.SysError("redemption failed: " + err.Error())
		return nil, ErrRedeemFailed
	}
	if result.RedeemType == RedemptionTypeSubscription && result.Subscription != nil {
		RecordLog(userId, LogTypeTopup, fmt.Sprintf("通过兑换码领取订阅，套餐: %s，兑换码ID %d", result.Subscription.PlanTitle, redemption.Id))
		return result, nil
	}
	RecordLog(userId, LogTypeTopup, fmt.Sprintf("通过兑换码充值 %s，兑换码ID %d", logger.LogQuota(redemption.Quota), redemption.Id))
	return result, nil
}

func (redemption *Redemption) Insert() error {
	var err error
	err = DB.Create(redemption).Error
	return err
}

func (redemption *Redemption) SelectUpdate() error {
	// This can update zero values
	return DB.Model(redemption).Select("redeemed_time", "status").Updates(redemption).Error
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (redemption *Redemption) Update() error {
	var err error
	err = DB.Model(redemption).Select("name", "status", "redeem_type", "quota", "plan_id", "redeemed_time", "expired_time").Updates(redemption).Error
	return err
}

func (redemption *Redemption) Delete() error {
	var err error
	err = DB.Delete(redemption).Error
	return err
}

func DeleteRedemptionById(id int) (err error) {
	if id == 0 {
		return errors.New("id 为空！")
	}
	redemption := Redemption{Id: id}
	err = DB.Where(redemption).First(&redemption).Error
	if err != nil {
		return err
	}
	return redemption.Delete()
}

func DeleteInvalidRedemptions() (int64, error) {
	now := common.GetTimestamp()
	result := DB.Where("status IN ? OR (status = ? AND expired_time != 0 AND expired_time < ?)", []int{common.RedemptionCodeStatusUsed, common.RedemptionCodeStatusDisabled}, common.RedemptionCodeStatusEnabled, now).Delete(&Redemption{})
	return result.RowsAffected, result.Error
}
