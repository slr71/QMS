package controllers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"

	"github.com/cyverse/QMS/internal/model"
	"github.com/labstack/echo/v4"
)

// GetAllPlans is the handler for the GET /v1/plans endpoint.
//
// swagger:route GET /v1/plans plans listPlans
//
// List Plans
//
// Lists all of the plans that are currently available.
//
// responses:
//   200: plansResponse
//   500: internalServerErrorResponse
func (s Server) GetAllPlans(ctx echo.Context) error {
	var data []model.Plan
	err := s.GORMDB.Debug().Find(&data).Error
	if err != nil {
		return model.Error(ctx, err.Error(), http.StatusInternalServerError)
	}
	return model.Success(ctx, data, http.StatusOK)
}

// GetPlanByID returns the plan with the given identifier.
//
// swagger:route GET /plans/{plan_id} plans getPlanByID
//
// Get Plan Information
//
// Returns the plan with the given identifier.
//
// responses:
//   200: planResponse
//   400: badRequestResponse
//   500: internalServerErrorResponse
func (s Server) GetPlanByID(ctx echo.Context) error {
	planId := ctx.Param("plan_id")
	if planId == "" {
		return model.Error(ctx, "invalid plan id", http.StatusBadRequest)
	}
	data := model.Plan{}
	err := s.GORMDB.Debug().Where("id=@id", sql.Named("id", planId)).Find(&data).Error
	if err != nil {
		return model.Error(ctx, err.Error(), http.StatusInternalServerError)
	}
	if data.Name == "" || data.Description == "" {
		msg := fmt.Sprintf("plan id not found: %s", planId)
		return model.Error(ctx, msg, http.StatusInternalServerError)
	}

	return model.Success(ctx, data, http.StatusOK)
}

type Plan struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type PlanDetail struct {
	PlanName    string `json:"plan_name"`
	Description string `json:"description"`
}

func (s Server) AddPlan(ctx echo.Context) error {
	var (
		err  error
		plan PlanDetail
	)
	fmt.Println(plan)
	if err = ctx.Bind(&plan); err != nil {
		return model.Error(ctx, err.Error(), http.StatusBadRequest)
	}
	fmt.Println(plan)

	if plan.PlanName == "" {
		return model.Error(ctx, "invalid plan name", http.StatusBadRequest)
	}
	if plan.Description == "" {
		return model.Error(ctx, "invalid plan description", http.StatusBadRequest)
	}
	var req = model.Plan{Name: plan.PlanName, Description: plan.Description}
	err = s.GORMDB.Debug().Create(&req).Error
	if err != nil {
		return model.Error(ctx, err.Error(), http.StatusInternalServerError)
	}
	return model.Success(ctx, "Success", http.StatusOK)
}

func (s Server) AddPlanQuotaDefault(ctx echo.Context) error {
	planName := "Basic"
	resourceName1 := "CPU"
	cpuValue := 4.00
	resourceName2 := "STORAGE"
	storageValue := 1000.00
	var plan = model.Plan{Name: planName}
	err := s.GORMDB.Debug().Find(&plan, "name=?", planName).Error
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError,
			model.ErrorResponse("plan name not found", http.StatusInternalServerError))
	}
	planId := *plan.ID
	var cpu = model.ResourceType{Name: resourceName1}
	err = s.GORMDB.Debug().Find(&cpu, "name=?", resourceName1).Error
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError,
			model.ErrorResponse("resource Type not found: "+resourceName1, http.StatusInternalServerError))
	}
	cpuId := *cpu.ID
	var req = model.PlanQuotaDefault{PlanID: &planId, ResourceTypeID: &cpuId, QuotaValue: cpuValue}
	err = s.GORMDB.Debug().Create(&req).Error
	if err != nil {
		return model.Error(ctx, err.Error(), http.StatusInternalServerError)
	}
	var storage = model.ResourceType{Name: resourceName2}
	err = s.GORMDB.Debug().Find(&storage, "name=?", resourceName2).Error
	if err != nil {
		return model.Error(ctx, "resource Type not found: "+resourceName2, http.StatusInternalServerError)
	}
	storageId := *storage.ID
	var req2 = model.PlanQuotaDefault{PlanID: &planId, ResourceTypeID: &storageId, QuotaValue: storageValue}
	err = s.GORMDB.Debug().Create(&req2).Error
	if err != nil {
		return model.Error(ctx, err.Error(), http.StatusInternalServerError)
	}
	return model.Success(ctx, "Success", http.StatusOK)
}

func (s Server) AddQuota(ctx echo.Context) error {
	username := ctx.Param("user_name")
	if username == "" {
		return model.Error(ctx, "invalid username", http.StatusBadRequest)
	}
	resourceName := ctx.Param("resource_name")
	if resourceName == "" {
		return model.Error(ctx, "invalid resource Name", http.StatusBadRequest)
	}
	quotaValue := ctx.Param("quota_value")
	if quotaValue == "" {
		return model.Error(ctx, "invalid Quota value", http.StatusBadRequest)
	}
	quotaValueFloat, err := ParseFloat(quotaValue)
	if err != nil {
		return model.Error(ctx, "invalid Quota Value", http.StatusInternalServerError)
	}
	var resource = model.ResourceType{Name: resourceName}
	err = s.GORMDB.Debug().Find(&resource, "name=?", resourceName).Error
	if err != nil {
		return model.Error(ctx, "resource Type not found: "+resourceName, http.StatusInternalServerError)
	}
	resourceID := *resource.ID
	var user = model.User{UserName: username}
	err = s.GORMDB.Debug().Find(&user, "user_name=?", username).Error
	if err != nil {
		return model.Error(ctx, "user name Not Found", http.StatusInternalServerError)
	}
	userID := *user.ID
	var userPlan = model.UserPlan{}
	err = s.GORMDB.Debug().
		Find(&userPlan, "user_id=?", userID).Error
	if err != nil {
		return model.Error(ctx, "user plan name not found for user: "+username, http.StatusInternalServerError)
	}
	userPlanId := *userPlan.ID
	var quota = model.Quota{
		UserPlanID:     &userPlanId,
		Quota:          quotaValueFloat,
		ResourceTypeID: &resourceID,
	}
	err = s.GORMDB.Debug().
		Create(&quota).Error
	if err != nil {
		return model.Error(ctx, err.Error(), http.StatusInternalServerError)
	}
	return model.Success(ctx, "Success", http.StatusOK)
}

func ParseFloat(valueString string) (float64, error) {
	valueFloat := 0.0
	if temp, err := strconv.ParseFloat(valueString, 64); err == nil {
		valueFloat = temp
	} else {
		return valueFloat, err
	}
	return valueFloat, nil
}
