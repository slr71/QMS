package controllers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cyverse/QMS/internal/model"
	"github.com/labstack/echo/v4"
)

const (
	UpdateTypeSet = "SET"
	UpdateTypeAdd = "ADD"
)

type UpdateUsagesReq struct {
	UserName             string  `json:"username"`
	ResourceType         string  `json:"resource_type"`
	UpdateType           string  `json:"update_type"`
	UsageAdjustmentValue float64 `json:"usage_adjustment_value"`
	EffectiveDate        string  `json:"effective_date"`
	Unit                 string  `json:"unit"`
}

type UpdateQuotaReq struct {
	Type  string  `json:"type"`
	Value float64 `json:"value"`
}

func (s Server) UpdateUsages(ctx echo.Context) error {
	var (
		err error
		req UpdateUsagesReq
	)
	if err = ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest,
			model.ErrorResponse(err.Error(), http.StatusBadRequest))
	}
	effectiveDate, err := time.Parse("2006-01-02", req.EffectiveDate)
	if err != nil {
		return model.Error(ctx, err.Error(), http.StatusBadRequest)
	}
	var resourceType = model.ResourceType{Name: req.ResourceType}
	err = s.GORMDB.Debug().Find(&resourceType).Error
	if err != nil {
		return model.Error(ctx, "resource type not found.", http.StatusInternalServerError)
	}
	resourceTypeID := *resourceType.ID
	var usageDetails []model.Usage
	err = s.GORMDB.Debug().
		Table("user_plans").
		Select("usages.*").
		Joins("JOIN usages ON user_plans.id=usages.user_plan_id").
		Joins("JOIN resource_types ON resource_types.id=usages.resource_type_id").
		Joins("JOIN quota ON user_plans.id=quota.user_plan_id").
		Joins("JOIN users ON users.id = user_plans.user_id").
		Where("resource_types.name=? AND users.user_name=?", req.ResourceType, req.UserName).
		Scan(&usageDetails).Error
	if err != nil {
		return model.Error(ctx, err.Error(), http.StatusInternalServerError)
	}
	for _, usageRec := range usageDetails {
		usageRec.UpdatedAt = effectiveDate
		value := req.UsageAdjustmentValue
		switch req.UpdateType {
		case UpdateTypeSet:
			usageRec.Usage = value
		case UpdateTypeAdd:
			usageRec.Usage += value
		default:
			msg := fmt.Sprintf("invalid update type: %s", req.UpdateType)
			return model.Error(ctx, msg, http.StatusBadRequest)
		}
		err := s.GORMDB.Debug().
			Updates(&usageRec).
			Where("resource_type_id=?", resourceTypeID).Error
		if err != nil {
			return model.Error(ctx, err.Error(), http.StatusInternalServerError)
		}
	}
	var updateOperation = model.UpdateOperation{}
	err = s.GORMDB.Debug().
		Where("name=?", req.UpdateType).
		Find(&updateOperation).
		Error
	if err != nil {
		return model.Error(ctx, err.Error(), http.StatusInternalServerError)
	}
	updateOperationID := *updateOperation.ID
	var update = model.Update{
		ValueType:         req.Unit,
		UpdatedBy:         "Admin",
		Value:             req.UsageAdjustmentValue,
		ResourceTypeID:    &resourceTypeID,
		UpdateOperationID: &updateOperationID,
	}
	err = s.GORMDB.Debug().Create(&update).Error
	if err != nil {
		return model.Error(ctx, err.Error(), http.StatusInternalServerError)
	}
	return model.Success(ctx, "Success", http.StatusOK)
}

func (s Server) GetAllActiveUsage(ctx echo.Context) error {
	var err error
	resource := ctx.QueryParam("resource_type")
	if resource == "" {
		return model.Error(ctx, "invalid resource name", http.StatusBadRequest)
	}
	username := ctx.QueryParam("username")
	if username == "" {
		return model.Error(ctx, "invalid username", http.StatusBadRequest)
	}
	var usageData []model.Usage
	usage := s.GORMDB.Debug().
		Joins("JOIN user_plans ON user_plans.id = usages.user_plan_id").
		Joins("JOIN resource_types ON resource_types.id = usages.resource_type_id").
		Joins("JOIN users ON users.id = user_plans.user_id").
		Where("cast(now() as date) between user_plans.effective_start_date and user_plans.effective_end_date")
	if username != "" {
		usage.Where("users.user_name = ?", username)
	}
	if resource != "" {
		usage.Where("resource_types.name = ?", resource)
	}
	if err = usage.Find(&usageData).Error; err != nil {
		return model.Error(ctx, err.Error(), http.StatusInternalServerError)
	}
	return model.Success(ctx, usageData, http.StatusOK)
}

func (s Server) GetAllUserActivePlans(ctx echo.Context) error {
	username := ctx.Param("username")
	if username == "" {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse("Invalid UserName", http.StatusBadRequest))
	}
	var planData []PlanDetails
	err := s.GORMDB.Raw(`select plans.name,usages.usage,quotas.quota,resource_types.unit from
	user_plans
	join plans on plans.id=user_plans.plan_id
	join usages on user_plans.id=usages.user_plan_id
	join quotas on user_plans.id=usages.user_plan_id
	join resource_types on resource_types.id=quotas.resource_type_id
	join users on users.id=user_plans.user_id
	where
	user_plans.effective_start_date<=cast(now() as date) and
	user_plans.effective_end_date>=cast(now() as date) ands
	users.username=?`, username).Scan(&planData).Error
	if err != nil {
		return model.Error(ctx, err.Error(), http.StatusInternalServerError)
	}
	return model.Success(ctx, planData, http.StatusOK)
}

func (s Server) AddUpdateOperation(ctx echo.Context) error {
	updateOperationName := ctx.Param("update_operation")
	if updateOperationName == "" {
		return model.Error(ctx, "invalid update operation", http.StatusBadRequest)
	}
	var updateOperation = model.UpdateOperation{Name: updateOperationName}
	err := s.GORMDB.Debug().Create(&updateOperation).Error
	if err != nil {
		return model.Error(ctx, err.Error(), http.StatusInternalServerError)
	}
	return model.Success(ctx, "Success", http.StatusOK)
}
