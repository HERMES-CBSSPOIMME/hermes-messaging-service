package router

import (
	json "encoding/json"
	errors "errors"
	fmt "fmt"
	log "log"
	http "net/http"
	auth "wave-messaging-management-service/auth"
	models "wave-messaging-management-service/models"
	utils "wave-messaging-management-service/utils"
	checkers "wave-messaging-management-service/validation/checkers"

	gocustomhttpresponse "github.com/terryvogelsang/gocustomhttpresponse"
	logruswrapper "github.com/terryvogelsang/logruswrapper"
)

type (
	// Handler : Custom type to work with CustomHandle wrapper
	Handler func(env *models.Env, w http.ResponseWriter, r *http.Request) error
)

// AddVerneMQACL : Construct and store VerneMQ ACL in database
func AddVerneMQACL(env *models.Env, w http.ResponseWriter, r *http.Request) error {

	// Retrieve token from request header
	token := r.Header.Get("token")

	// Check if token has valid format (According to regex provided by environment variable)
	tokenHasValidFormat, err := checkers.IsTokenValid(env, token)

	if err != nil {
		return err
	}

	// If token is not formatted correctly, return an error response
	if !tokenHasValidFormat {
		log.Println("Invalid token format")
		return errors.New(logruswrapper.CodeInvalidToken)
	}

	// Check authentication with provided endpoint
	MQTTAuthInfos, wasCached, wasTokenUpdated, err := auth.CheckAuthentication(env, token)

	// If an error occurs, token is invalid
	if err != nil {
		log.Println(err)
		return errors.New(logruswrapper.CodeInvalidToken)
	}

	if wasTokenUpdated {
		log.Println("Token Updated")
		return errors.New(logruswrapper.CodeUpdated)
	}

	if wasCached {
		log.Println("Already cached")
		return errors.New(logruswrapper.CodeAlreadyExists)
	}

	// Construct MQTT User ACL with MQTT Auth Infos + default ACLs
	verneMQACL := models.NewVerneMQACL(MQTTAuthInfos.ClientID, MQTTAuthInfos.Username, MQTTAuthInfos.Password)

	err = env.MongoDB.AddProfileACL(verneMQACL)

	if err != nil {
		log.Println(err)
		return errors.New(logruswrapper.CodeInvalidToken)
	}

	log := logruswrapper.NewEntry("MessagingService", "/helloworld", logruswrapper.CodeSuccess)

	gocustomhttpresponse.WriteResponse(MQTTAuthInfos.ClientID, log, w)
	return nil
}

// AddGroupConversation : Add group conversation ACLs in database
func AddGroupConversation(env *models.Env, w http.ResponseWriter, r *http.Request) error {

	// Retrieve token from request header
	token := r.Header.Get("token")

	// Check if token has valid format (According to regex provided by environment variable)
	tokenHasValidFormat, err := checkers.IsTokenValid(env, token)

	if err != nil {
		return err
	}

	// If token is not formatted correctly, return an error response
	if !tokenHasValidFormat {
		return errors.New(logruswrapper.CodeInvalidToken)
	}

	// Check authentication with provided endpoint
	MQTTAuthInfos, _, _, err := auth.CheckAuthentication(env, token)

	// If an error occurs, token is invalid
	if err != nil {
		return errors.New(logruswrapper.CodeInvalidToken)
	}

	reqBody := utils.GroupConversationBody{}
	err = json.NewDecoder(r.Body).Decode(&reqBody)

	if err != nil {
		return errors.New(logruswrapper.CodeInvalidJSON)
	}

	// create a zero-length slice with the same underlying array
	tmp := reqBody.Members[:0]

	// Check if provided users exist, if not do not store it in DB
	for _, member := range reqBody.Members {

		doesExist, err := env.Redis.Exists("mapping:" + member)

		if err != nil {
			// TODO: Add code an error occured
			fmt.Println(err)
			return errors.New(logruswrapper.CodeInvalidJSON)
		}

		// If user does not exists, remove from mapping
		if doesExist {

			internalWaveUserID, err := env.Redis.HGet("mapping:"+member, "internalWaveUserID")

			if err != nil {
				// TODO: Add code an error occured
				fmt.Println(err)
				return errors.New(logruswrapper.CodeInvalidJSON)
			}

			// Remove potential duplicate of emitter user ID
			if string(internalWaveUserID) != MQTTAuthInfos.ClientID {
				tmp = append(tmp, string(internalWaveUserID))
			}
		}
	}

	// Set group conversation with existing members
	reqBody.Members = tmp

	// Create new group conversation struct
	groupConv := models.NewGroupConversation(reqBody.Name, append(reqBody.Members, MQTTAuthInfos.ClientID))

	// Store conversation infos in DB
	err = env.MongoDB.AddGroupConversation(groupConv)

	// Update ACL in DB (Request maker get publish rights on recipient private topic)
	err = env.MongoDB.UpdateProfilesWithGroupACL(groupConv)

	if err != nil {
		return errors.New(logruswrapper.CodeInvalidToken)
	}

	log := logruswrapper.NewEntry("MessagingService", "/helloworld", logruswrapper.CodeSuccess)

	gocustomhttpresponse.WriteResponse(nil, log, w)
	return nil
}

// CustomHandle : Custom Handlers Wrapper for API
func CustomHandle(env *models.Env, handlers ...Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, h := range handlers {
			err := h(env, w, r)
			if err != nil {
				errorLog := logruswrapper.NewEntry("MessagingService", "/helloworld", err.Error())
				gocustomhttpresponse.WriteResponse(nil, errorLog, w)
				return
			}
		}
	})
}

// GetMappingForUsers : Get internal wave user IDs
func GetMappingForUsers(env *models.Env, w http.ResponseWriter, r *http.Request) error {

	// Retrieve token from request header
	token := r.Header.Get("token")

	// Check if token has valid format (According to regex provided by environment variable)
	tokenHasValidFormat, err := checkers.IsTokenValid(env, token)

	if err != nil {
		return err
	}

	// If token is not formatted correctly, return an error response
	if !tokenHasValidFormat {
		return errors.New(logruswrapper.CodeInvalidToken)
	}

	// Check authentication with provided endpoint
	_, _, _, err = auth.CheckAuthentication(env, token)

	// If an error occurs, token is invalid
	if err != nil {
		return errors.New(logruswrapper.CodeInvalidToken)
	}

	reqBody := utils.MappingRequestBody{}

	err = json.NewDecoder(r.Body).Decode(&reqBody)

	if err != nil {
		return errors.New(logruswrapper.CodeInvalidJSON)
	}

	mappings := []models.Mapping{}

	for _, userID := range reqBody.UserIDs {

		internalWaveUserID, _ := env.Redis.HGet("mapping:"+userID, "internalWaveUserID")

		fmt.Println(string(internalWaveUserID))
		if string(internalWaveUserID) != "" {
			mappings = append(mappings, models.Mapping{OriginalUserID: userID, InternalWaveUserID: string(internalWaveUserID)})
		}
	}

	log := logruswrapper.NewEntry("MessagingService", "/profiles/mappings", logruswrapper.CodeSuccess)

	gocustomhttpresponse.WriteResponse(mappings, log, w)

	return nil
}
