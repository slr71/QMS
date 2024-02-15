package controllers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/cyverse-de/go-mod/pbinit"
	"github.com/cyverse-de/p/go/qms"
	"github.com/cyverse/QMS/internal/db"
	"github.com/cyverse/QMS/internal/httpmodel"
	"github.com/cyverse/QMS/internal/model"
	"github.com/cyverse/QMS/internal/query"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
)

const (
	UpdateTypeSet = "SET"
	UpdateTypeAdd = "ADD"
)

// swagger:route GET /v1/users users listUsers
//
// List Users
//
// Lists the users registered in the QMS database.
//
// responses:
//   200: userListing
//   500: internalServerErrorResponse

// GetAllUsers lists the users that are currently defined in the database.
func (s Server) GetAllUsers(ctx echo.Context) error {
	var data []model.User
	err := s.GORMDB.Debug().Find(&data).Error
	if err != nil {
		return model.Error(ctx, err.Error(), http.StatusInternalServerError)
	}
	return ctx.JSON(http.StatusOK, model.SuccessResponse(data, http.StatusOK))
}

type Result struct {
	ID             *string
	UserName       string
	ResourceTypeID *string
}

// GetSubscriptionDetails returns information about the currently active plan for the user.
func (s Server) GetSubscriptionDetails(ctx echo.Context) error {
	log := log.WithFields(logrus.Fields{"context": "getting active user plan"})

	context := ctx.Request().Context()

	username := strings.TrimSuffix(ctx.Param("username"), s.UsernameSuffix)
	if username == "" {
		return model.Error(ctx, "invalid username", http.StatusBadRequest)
	}

	log = log.WithFields(logrus.Fields{"user": username})

	// Start a transaction.
	return s.GORMDB.Transaction(func(tx *gorm.DB) error {
		var err error

		// Look up or insert the user.
		user, err := db.GetUser(context, tx, username)
		if err != nil {
			return model.Error(ctx, err.Error(), http.StatusInternalServerError)
		}

		log.Debugf("found user %s in db", user.Username)

		// Look up or create the user plan.
		subscription, err := db.GetActiveSubscriptionDetails(context, tx, user.Username)
		if err != nil {
			return model.Error(ctx, err.Error(), http.StatusInternalServerError)
		}
		log.Debugf("user plan name is %s", subscription.Plan.Name)

		// Return the user plan.
		return model.Success(ctx, subscription, http.StatusOK)
	})
}

// swagger:route POST /v1/users/:username/plan/:resource-type/quota users updateCurrentSubscriptionQuota
//
// Update Current Subscription Plan Quota
//
// Updates the current quota for the given username and resource type. If the user doesn't have an active
// subscription then a new subscription for the default subscription plan type will be created.
//
// responses:
//   200: subscriptionsResponse
//   400: badRequestResponse
//   500: internalServerErrorResponse

// UpdateCurrentSubscriptionQuota is the handler for updating the quota associated with a user's current
// subscription plan.
func (s Server) UpdateCurrentSubscriptionQuota(c echo.Context) error {
	log := log.WithField("context", "updating a current subscription quota")
	ctx := c.Request().Context()

	// Extract the username from the request.
	username := strings.TrimSuffix(c.Param("username"), s.UsernameSuffix)
	if username == "" {
		msg := fmt.Sprintf("invalid username provided in request: '%s'", c.Param("username"))
		log.Error(msg)
		return model.Error(c, msg, http.StatusBadRequest)
	}
	log = log.WithField("user", username)

	// Extract the resource type name from the request.
	resourceTypeName := c.Param("resource-type")
	if resourceTypeName == "" {
		msg := "no resource type name provided in request"
		log.Error(msg)
		return model.Error(c, msg, http.StatusBadRequest)
	}
	log = log.WithField("resource-type", resourceTypeName)

	// Parse the request body.
	var body httpmodel.QuotaValue
	err := c.Bind(&body)
	if err != nil {
		msg := fmt.Sprintf("invalid request body: %s", err.Error())
		log.Error(msg)
		return model.Error(c, msg, http.StatusBadRequest)
	}
	if err = c.Validate(&body); err != nil {
		msg := fmt.Sprintf("invalid request body: %s", err.Error())
		log.Error(msg)
		return model.Error(c, msg, http.StatusBadRequest)
	}

	// Start a transaction.
	return s.GORMDB.Transaction(func(tx *gorm.DB) error {
		// Look up the resource type.
		resourceType, err := db.GetResourceTypeByName(ctx, tx, resourceTypeName)
		if err != nil {
			log.Error(err)
			return model.Error(c, err.Error(), http.StatusInternalServerError)
		}
		if resourceType == nil {
			msg := fmt.Sprintf("resource type '%s' not found", resourceTypeName)
			log.Error(msg)
			return model.Error(c, msg, http.StatusBadRequest)
		}

		// Determine whether or not the user has an active subscription.
		hasActiveSubscription, err := db.HasActiveSubscription(ctx, tx, username)
		if err != nil {
			log.Error(err)
			return model.Error(c, err.Error(), http.StatusInternalServerError)
		}

		// Load the user's current subscription, creating a new subscription if necessary.
		subcription, err := db.GetActiveSubscription(ctx, tx, username)
		if err != nil {
			log.Error(err)
			return model.Error(c, err.Error(), http.StatusInternalServerError)
		}

		// Insert or update the quota.
		quota := &model.Quota{
			SubscriptionID: subcription.ID,
			Quota:          body.Quota,
			ResourceTypeID: resourceType.ID,
		}
		err = db.UpsertQuota(ctx, tx, quota)
		if err != nil {
			log.Error(err)
			return model.Error(c, err.Error(), http.StatusInternalServerError)
		}

		// Load the subscription details.
		details, err := db.GetSubscriptionDetails(ctx, tx, *subcription.ID)
		if err != nil {
			log.Error(err)
			return model.Error(c, err.Error(), http.StatusInternalServerError)
		}

		// Return the response.
		responseBody := model.SubscriptionResponseFromSubscription(details, !hasActiveSubscription)
		return model.Success(c, responseBody, http.StatusOK)
	})
}

// GetUserOverages is the echo handler for listing the resources that a user is
// in overage for.
func (s Server) GetUserOverages(ctx echo.Context) error {
	log := log.WithFields(logrus.Fields{"context": "getting any overages for the user"})

	context := ctx.Request().Context()

	responseList := pbinit.NewOverageList()

	// Skip the remaining logic because QMS is configured to not report overages.
	if !s.ReportOverages {
		return model.ProtobufJSON(ctx, responseList, http.StatusOK)
	}

	username := strings.TrimSuffix(ctx.Param("username"), s.UsernameSuffix)
	if username == "" {
		return model.Error(ctx, "missing username", http.StatusBadRequest)
	}

	log.WithFields(logrus.Fields{"user": username})

	log.Info("looking up any overages")

	log.Debug("before calling db.GetUserOverages()")
	results, err := db.GetUserOverages(context, s.GORMDB, username)
	if err != nil {
		return model.Error(ctx, err.Error(), http.StatusInternalServerError)
	}
	log.Debug("after calling db.GetUserOverages()")

	for _, r := range results {
		responseList.Overages = append(responseList.Overages, &qms.Overage{
			ResourceName: r["resource_type_name"].(string),
			Quota:        r["quota"].(float64),
			Usage:        r["usage"].(float64),
		})
	}

	return model.ProtobufJSON(ctx, responseList, http.StatusOK)
}

// InOverage is the echo handler for checking if a user is in overage for a
// resource.
func (s Server) InOverage(ctx echo.Context) error {
	log := log.WithFields(logrus.Fields{"context": "checking if a user's usage is an overage"})

	context := ctx.Request().Context()

	response := pbinit.NewIsOverage()

	// Skip the rest of the logic because QMS is configured to not report overages
	if !s.ReportOverages {
		response.IsOverage = false
		return model.ProtobufJSON(ctx, response, http.StatusOK)
	}

	username := strings.TrimSuffix(ctx.Param("username"), s.UsernameSuffix)
	if username == "" {
		return model.Error(ctx, "missing username", http.StatusBadRequest)
	}

	resource := ctx.Param("resource-name")
	if resource == "" {
		return model.Error(ctx, "missing resource name", http.StatusBadRequest)
	}

	log.WithFields(logrus.Fields{"user": username, "resource": resource})

	log.Info("checking if the usage is an overage")

	results, err := db.IsOverage(context, s.GORMDB, username, resource)
	if err != nil {
		return model.Error(ctx, err.Error(), http.StatusInternalServerError)
	}

	if results != nil {
		response.IsOverage = results["overage"].(bool)
	}

	return model.ProtobufJSON(ctx, response, http.StatusOK)
}

// AddUser adds a new user to the database. This is a no-op if the user already
// exists.
func (s Server) AddUser(ctx echo.Context) error {
	log := log.WithFields(logrus.Fields{"context": "adding user"})

	context := ctx.Request().Context()

	username := strings.TrimSuffix(ctx.Param("username"), s.UsernameSuffix)
	if username == "" {
		return model.Error(ctx, "invalid username", http.StatusBadRequest)
	}

	log.Debugf("user from request is %s", username)

	log = log.WithFields(logrus.Fields{"user": username})

	// Start a transaction.
	return s.GORMDB.Transaction(func(tx *gorm.DB) error {
		var err error

		// Either add the user to the database or look up the existing user
		// information.
		user, err := db.GetUser(context, tx, username)
		if err != nil {
			return model.Error(ctx, err.Error(), http.StatusInternalServerError)
		}

		log.Debug("found user in the database")

		// GetActiveSubscription will automatically subscribe the user to the basic
		// plan if not subscribed already.
		_, err = db.GetActiveSubscription(context, tx, user.Username)
		if err != nil {
			return model.Error(ctx, err.Error(), http.StatusInternalServerError)
		}

		log.Debug("ensured that user is subscribed to a plan")

		return model.Success(ctx, "Success", http.StatusOK)
	})
}

// swagger:route PUT /v1/users/{username}/{plan_name} users updateSubscription
//
// # Subscribe a User to a New Plan
//
// Creates a new subscription for the user with the given username.
//
// Responses:
//   200: subscription
//   400: badRequestResponse
//   500: internalServerErrorResponse

// UpdateSubscription is the handler for the PUT /v1/users/{username}/{plan_name} endpoint.
func (s Server) UpdateSubscription(ctx echo.Context) error {
	log := log.WithFields(logrus.Fields{"context": "updating user plan"})

	context := ctx.Request().Context()

	planName := ctx.Param("plan_name")
	if planName == "" {
		return model.Error(ctx, "invalid plan name", http.StatusBadRequest)
	}
	log.Debugf("plan name from request is %s", planName)

	paid, err := query.ValidateBooleanQueryParam(ctx, "paid", nil)
	if err != nil {
		return model.Error(ctx, err.Error(), http.StatusBadRequest)
	}
	log.Debugf("paid flag from request is %t", paid)

	username := strings.TrimSuffix(ctx.Param("username"), s.UsernameSuffix)
	if username == "" {
		return model.Error(ctx, "invalid username", http.StatusBadRequest)
	}
	log.Debugf("user name from request is %s", username)

	var defaultPeriods int32 = 1
	periods, err := query.ValidateIntQueryParam(ctx, "periods", &defaultPeriods, "gte=0")
	if err != nil {
		return model.Error(ctx, err.Error(), http.StatusBadRequest)
	}
	log.Debugf("periods from request is %d", periods)

	defaultEndDate := time.Now().AddDate(1, 0, 0)
	endDate, err := query.ValidateDateQueryParam(ctx, "end_date", &defaultEndDate)
	if err != nil {
		return model.Error(ctx, err.Error(), http.StatusBadRequest)
	}
	if !endDate.After(time.Now()) {
		return model.Error(ctx, "end date must be in the future", http.StatusBadRequest)
	}
	log.Debugf("end date from request is %s", endDate)

	log = log.WithFields(logrus.Fields{
		"user": username,
		"plan": planName,
	})

	// Start a transaction.
	return s.GORMDB.Transaction(func(tx *gorm.DB) error {
		var err error

		// Either add the user to the database or look up the existing user information.
		user, err := db.GetUser(context, tx, username)
		if err != nil {
			return model.Error(ctx, err.Error(), http.StatusInternalServerError)
		}

		log.Debug("found user in the database")

		// Verify that a plan with the given name exists.
		plan, err := db.GetPlan(context, tx, planName)
		if err != nil {
			return model.Error(ctx, err.Error(), http.StatusInternalServerError)
		}
		if plan == nil {
			msg := fmt.Sprintf("plan name `%s` not found", planName)
			return model.Error(ctx, msg, http.StatusBadRequest)
		}

		log.Debug("verified that plan exists in database")

		// Deactivate all active plans for the user.
		err = db.DeactivateSubscriptions(context, tx, *user.ID)
		if err != nil {
			return model.Error(ctx, err.Error(), http.StatusInternalServerError)
		}

		log.Debug("deactivated all active plans for the user")

		// Subscribe the user to the plan.
		subscription, err := db.SubscribeUserToPlan(context, tx, user, plan, paid)
		if err != nil {
			return model.Error(ctx, err.Error(), http.StatusInternalServerError)
		}

		log.Debug("subscribed user to the new plan")

		// Load the subscription details.
		details, err := db.GetSubscriptionDetails(context, tx, *subscription.ID)
		if err != nil {
			log.Error(err)
			return model.Error(ctx, err.Error(), http.StatusInternalServerError)
		}

		// Return the response.
		return model.Success(ctx, details, http.StatusOK)
	})
}
