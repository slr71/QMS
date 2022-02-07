package controllers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/cyverse/QMS/internal/model"
	"github.com/labstack/echo"
)

// swagger:route GET /plans plans listPlans
// Returns a List all the plans
// responses:
//   200: plansResponse
//   404: RootResponse
func (s Server) GetAllPlans(ctx echo.Context) error {
	data := []model.Plan{}
	err := s.GORMDB.Debug().Find(&data).Error
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, model.ErrorResponse(err.Error(), http.StatusInternalServerError))
	}
	return ctx.JSON(http.StatusOK, model.SuccessResponse(data, http.StatusOK))
}

// swagger:route GET /plans/{PlanID} plans listPlansByID
// Returns a List all the plans
// responses:
//   200: plansResponse
//   500: RootResponse
func (s Server) GetPlansForID(ctx echo.Context) error {
	plan_id := ctx.Param("plan_id")
	if plan_id == "" {
		return ctx.JSON(http.StatusInternalServerError, model.ErrorResponse("Invalid PlanID", http.StatusInternalServerError))
	}
	data := model.Plan{}
	err := s.GORMDB.Debug().Where("id=@id", sql.Named("id", plan_id)).Find(&data).Error
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, model.ErrorResponse(err.Error(), http.StatusInternalServerError))
	}
	if data.Name == "" || data.Description == "" {
		return ctx.JSON(http.StatusInternalServerError, model.ErrorResponse("Invalid PlanID", http.StatusInternalServerError))
	}

	return ctx.JSON(http.StatusOK, model.SuccessResponse(data, http.StatusOK))
}

type Plan struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s Server) AddPlan(ctx echo.Context) error {
	planname := ctx.Param("plan_name")
	if planname == "" {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse("Invalid Plan Name", http.StatusBadRequest))
	}
	description := ctx.Param("description")
	if description == "" {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse("Invalid Plan description", http.StatusBadRequest))
	}
	var req = model.Plan{Name: planname, Description: description}
	err := s.GORMDB.Debug().Create(&req).Error
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, model.ErrorResponse(err.Error(), http.StatusInternalServerError))
	}
	return ctx.JSON(http.StatusOK, model.SuccessResponse("Success", http.StatusOK))
}

func (s Server) AddResourceType(ctx echo.Context) error {
	name := ctx.Param("resource_name")
	if name == "" {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse("Invalid Resource Name", http.StatusBadRequest))
	}
	unit := ctx.Param("resource_unit")
	if unit == "" {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse("Invalid Resource Unit", http.StatusBadRequest))
	}
	var resource_type = model.ResourceType{Name: name, Unit: unit}
	err := s.GORMDB.Debug().Create(&resource_type).Error
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, model.ErrorResponse(err.Error(), http.StatusInternalServerError))
	}
	return ctx.JSON(http.StatusOK, model.SuccessResponse("Success", http.StatusOK))
}

func (s Server) AddPlanQuotaDefault(ctx echo.Context) error {
	planName := "Basic"
	resourceName1 := "CPU"
	cpuValue := 4.00
	resourceName2 := "STORAGE"
	storageValue := 1000.00
	var plan = model.Plan{Name: planName}
	s.GORMDB.Debug().Find(&plan, "name=?", planName)
	planId := *plan.ID
	var cpu = model.ResourceType{Name: resourceName1}
	s.GORMDB.Debug().Find(&cpu, "name=?", resourceName1)
	cpuId := *cpu.ID
	var req = model.PlanQuotaDefault{PlanID: &planId, ResourceTypeID: &cpuId, QuotaValue: cpuValue}
	err := s.GORMDB.Debug().Create(&req).Error
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, model.ErrorResponse(err.Error(), http.StatusInternalServerError))
	}
	var storage = model.ResourceType{Name: resourceName2}
	s.GORMDB.Debug().Find(&storage, "name=?", resourceName2)
	storageId := *storage.ID
	var req2 = model.PlanQuotaDefault{PlanID: &planId, ResourceTypeID: &storageId, QuotaValue: storageValue}
	err = s.GORMDB.Debug().Create(&req2).Error
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, model.ErrorResponse(err.Error(), http.StatusInternalServerError))
	}
	return ctx.JSON(http.StatusOK, model.SuccessResponse("Success", http.StatusOK))
}

func (s Server) UpdateUserPlanDetails(ctx echo.Context) error {
	planname := ctx.Param("plan_name")
	if planname == "" {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse("Invalid Plan Name", http.StatusBadRequest))
	}
	username := ctx.Param("user_name")
	if username == "" {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse("Invalid UserName", http.StatusBadRequest))
	}
	var user = model.User{UserName: username}
	s.GORMDB.Debug().Find(&user, "user_name=?", username)
	userID := *user.ID

	var plan = model.Plan{Name: planname}
	s.GORMDB.Debug().Find(&plan, "name=?", planname)
	planId := *plan.ID

	var req = model.UserPlan{AddedBy: "Admin", LastModifiedBy: "Admin", UserID: &userID}
	err := s.GORMDB.Debug().Model(&req).Where("user_id=?", userID).Update("plan_id", planId).Error
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, model.ErrorResponse(err.Error(), http.StatusInternalServerError))
	}
	return ctx.JSON(http.StatusOK, model.SuccessResponse("Success", http.StatusOK))
}

func (s Server) AddQuota(ctx echo.Context) error {
	username := ctx.Param("user_name")
	if username == "" {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse("Invalid UserName", http.StatusBadRequest))
	}
	resourceName := ctx.Param("resource_name")
	if resourceName == "" {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse("Invalid resource Name", http.StatusBadRequest))
	}
	quotaValue := ctx.Param("quota_value")
	if quotaValue == "" {
		return ctx.JSON(http.StatusBadRequest, model.ErrorResponse("Invalid Quota value", http.StatusBadRequest))
	}
	quotaValueFloat := ParseFloat(quotaValue)
	var resource = model.ResourceType{Name: resourceName}
	s.GORMDB.Debug().Find(&resource, "name=?", resourceName)
	resourceID := *resource.ID

	var user = model.User{UserName: username}
	s.GORMDB.Debug().Find(&user, "user_name=?", username)
	userID := *user.ID
	var userPlan = model.UserPlan{}
	s.GORMDB.Debug().Find(&userPlan, "user_id=?", userID)
	userPlanId := *userPlan.ID
	var req = model.Quota{UserPlanID: &userPlanId, Quota: quotaValueFloat, ResourceTypeID: &resourceID}
	err := s.GORMDB.Debug().Create(&req).Error
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, model.ErrorResponse(err.Error(), http.StatusInternalServerError))
	}
	return ctx.JSON(http.StatusOK, model.SuccessResponse("Success", http.StatusOK))
}

func ParseFloat(valueString string) float64 {
	valueFloat := 0.0
	if temp, err := strconv.ParseFloat(valueString, 64); err == nil {
		valueFloat = temp
	}
	return valueFloat
}
