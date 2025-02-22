package models

import (
	context "context"
	utils "wave-messaging-management-service/utils"

	mongoBSON "github.com/mongodb/mongo-go-driver/bson"
	mongo "github.com/mongodb/mongo-go-driver/mongo"
	bson "gopkg.in/mgo.v2/bson"
)

const (

	// WaveDatabaseName : Database name of the Wave project as defined in MongoDB
	WaveDatabaseName = "waveDB"

	// PrivateConversationsCollection : MongoDB Collection containing private conversations backups
	PrivateConversationsCollection = "privateConversations"

	// VerneMQACLCollection : MongoDB Collection containing VerneMQ ACLs
	VerneMQACLCollection = "vmq_acl_auth"

	// GroupConversationCollection : MongoDB Collection containing group private conversations backups
	GroupConversationCollection = "groupConversations"
)

// MongoDBInterface : MongoDB Communication interface
type MongoDBInterface interface {
	AddGroupConversation(groupConversation *GroupConversation) error
	AddProfileACL(verneMQACL *VerneMQACL) error
	AuthorizePublishing(userID string, topic string) error
	UpdateProfilesWithGroupACL(groupConversation *GroupConversation) error
	UpdatePassHash(userID string, newPasshash string) error
}

// MongoDB : MongoDB communication interface
type MongoDB struct {
	Client                         *mongo.Client
	WaveDB                         *mongo.Database
	PrivateConversationsCollection *mongo.Collection
	VerneMQACLCollection           *mongo.Collection
	GroupConversationCollection    *mongo.Collection
}

// NewMongoDB : Return a new MongoDB abstraction struct
func NewMongoDB(connectionURL string) *MongoDB {

	// Get connection to DB
	client, err := mongo.NewClient(connectionURL)

	if err != nil {
		utils.PanicOnError(err, "Failed to connect to MongoDB")
	}

	err = client.Connect(context.TODO())

	if err != nil {
		utils.PanicOnError(err, "Failed to connect to context")
	}

	// Get database reference
	waveDB := client.Database(WaveDatabaseName)

	// Get collections references
	privateConversationsCollection := waveDB.Collection(PrivateConversationsCollection)
	vmqACLCollection := waveDB.Collection(VerneMQACLCollection)
	groupConversationCollection := waveDB.Collection(GroupConversationCollection)

	// Return new MongoDB abstraction struct
	return &MongoDB{
		Client:                         client,
		WaveDB:                         waveDB,
		PrivateConversationsCollection: privateConversationsCollection,
		VerneMQACLCollection:           vmqACLCollection,
		GroupConversationCollection:    groupConversationCollection,
	}
}

// AddGroupConversation : Add group conversation entry in database
func (mongoDB *MongoDB) AddGroupConversation(groupConversation *GroupConversation) error {

	// Marshal struct into bson object
	doc, err := bson.Marshal(*groupConversation)

	if err != nil {
		return err
	}

	// Insert group conversation into DB
	_, err = mongoDB.GroupConversationCollection.InsertOne(nil, doc)

	if err != nil {
		return err
	}

	return nil
}

// AddProfileACL : Add VerneMQ ACL for user in database
// Should be trigerred when a user connect for the first time
func (mongoDB *MongoDB) AddProfileACL(verneMQACL *VerneMQACL) error {

	// Marshal struct into bson object
	doc, err := bson.Marshal(*verneMQACL)

	if err != nil {
		return err
	}

	// Insert ACL into VerneMQ ACL Collection
	_, err = mongoDB.VerneMQACLCollection.InsertOne(nil, doc)

	if err != nil {
		return err
	}

	return nil
}

// AuthorizePublishing : Authorize publishing on MQTT topic for userID
func (mongoDB *MongoDB) AuthorizePublishing(userID string, topic string) error {

	_, err := mongoDB.VerneMQACLCollection.UpdateOne(
		nil,
		mongoBSON.NewDocument(
			mongoBSON.EC.String("client_id", userID),
		),
		mongoBSON.NewDocument(
			mongoBSON.EC.SubDocumentFromElements("$push",
				mongoBSON.EC.String("publish_acl", topic),
			),
		),
	)
	if err != nil {
		return err
	}
	return nil
}

// UpdateProfilesWithGroupACL : Update VerneMQ Acls in database to grant publish and read access to all members of the group
func (mongoDB *MongoDB) UpdateProfilesWithGroupACL(groupConversation *GroupConversation) error {

	for _, userID := range groupConversation.Members {

		_, err := mongoDB.VerneMQACLCollection.UpdateOne(
			nil,
			mongoBSON.NewDocument(
				mongoBSON.EC.String("client_id", userID),
			),
			mongoBSON.NewDocument(
				mongoBSON.EC.SubDocumentFromElements("$push",
					mongoBSON.EC.SubDocumentFromElements("publish_acl",
						mongoBSON.EC.String("pattern", GroupConversationTopicPath+groupConversation.GroupConversationID+"/"+userID)),
				),
				mongoBSON.EC.SubDocumentFromElements("$push",
					mongoBSON.EC.SubDocumentFromElements("subscribe_acl",
						mongoBSON.EC.String("pattern", GroupConversationTopicPath+groupConversation.GroupConversationID+"/+")),
				),
			),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// UpdatePassHash : Update passhash field in VerneMQ ACLs Collection Acls
func (mongoDB *MongoDB) UpdatePassHash(userID string, newPasshash string) error {

	_, err := mongoDB.VerneMQACLCollection.UpdateOne(
		nil,
		mongoBSON.NewDocument(
			mongoBSON.EC.String("client_id", userID),
		),
		mongoBSON.NewDocument(
			mongoBSON.EC.SubDocumentFromElements("$set",
				mongoBSON.EC.String("passhash", newPasshash),
			),
		),
	)
	if err != nil {
		return err
	}

	return nil
}
