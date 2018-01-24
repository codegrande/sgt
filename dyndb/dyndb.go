package dyndb

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/oktasecuritylabs/sgt/logger"
	osq_types "github.com/oktasecuritylabs/sgt/osquery_types"
)
//DbInstance creates a new pointer to dynamodb from assumed role by ec2 instance
func DbInstance() *dynamodb.DynamoDB {
	sess := session.Must(session.NewSession(
		&aws.Config{
			Region: aws.String("us-east-1"),
		}))
	creds := credentials.NewChainCredentials(
		[]credentials.Provider{
			&credentials.EnvProvider{},
			&ec2rolecreds.EC2RoleProvider{
				Client: ec2metadata.New(sess),
			},
		})
	dynamoDB := dynamodb.New(session.Must(session.NewSession(&aws.Config{
		Region:      aws.String("us-east-1"),
		Credentials: creds,
	})))
	return dynamoDB
}

//BuildOsqueryPacksAsJSON returns raw json of a named config
func BuildOsqueryPacksAsJSON(nc osq_types.OsqueryNamedConfig) json.RawMessage {
	dynamoDB := DbInstance()
	packJSON := "{"
	var packList []string
	for i, pack := range nc.PackList {
		logger.Debug(pack, i)
		p, err := GetNewPackByName(pack, dynamoDB)
		if err != nil {
			logger.Error(err)
		}
		packList = append(packList, p.AsString())
	}
	packJSON += strings.Join(packList, ", ")
	packJSON += "}"
	return json.RawMessage(packJSON)
}

//UpsertNamedConfig upserts named config to dynamo db.  Returns true if successful, else false
func UpsertNamedConfig(dynamoDB *dynamodb.DynamoDB, onc *osq_types.OsqueryNamedConfig, mut sync.Mutex) bool {
	av, err := dynamodbattribute.MarshalMap(onc)
	//fmt.Println(av)
	if err != nil {
		logger.Info("Marshal Failed")
		logger.Error(err)
	}
	_, err = dynamoDB.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String("osquery_configurations"),
		Item:      av,
	})
	if err != nil {
		fmt.Println(err)
		return false
	}
	return true
}

//GetNamedConfigs returns all named configs
func GetNamedConfigs(dynamoDB *dynamodb.DynamoDB) ([]osq_types.OsqueryNamedConfig, error) {
	results := []osq_types.OsqueryNamedConfig{}
	scanItems, err := dynamoDB.Scan(&dynamodb.ScanInput{
		TableName: aws.String("osquery_configurations"),
	})
	if err != nil {
		logger.Error(err)
		return results, err
	}
	for _, i := range scanItems.Items {
		config := osq_types.OsqueryNamedConfig{}
		err = dynamodbattribute.UnmarshalMap(i, &config)
		if err != nil {
			logger.Error(err)
			return results, err
		}
		results = append(results, config)
	}
	return results, nil
}

//GetNamedConfig returns named config specified by string configName
func GetNamedConfig(dynamoDB *dynamodb.DynamoDB, configName string) (osq_types.OsqueryNamedConfig, error) {
	namedConfig := osq_types.OsqueryNamedConfig{}
	if configName == "" {
		return namedConfig, errors.New("no config name specified")
	}
	type querystring struct {
		ConfigName string `json:"config_name"`
	}
	var qs querystring
	qs.ConfigName = configName
	//if config name is not "", return specified config(if exists)
	//fmt.Println(qs)
	js, err := dynamodbattribute.MarshalMap(qs)
	if err != nil {
		logger.Error(err)
		return namedConfig, err
	}
	//fmt.Println(js)
	resp, err := dynamoDB.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String("osquery_configurations"),
		Key:       js,
	})
	if err != nil {
		logger.Warn("[dyndb.GetNamedConfig] ", err)
		return namedConfig, err
	}
	if len(resp.Item) > 0 {
		err = dynamodbattribute.UnmarshalMap(resp.Item, &namedConfig)
		if err != nil {
			logger.Warn("UNMARSHAL ERROR")
			return namedConfig, err
		}
		return namedConfig, nil
	}
	return namedConfig, nil
}

//UpsertClient upsers an osqueryClient
func UpsertClient(oc osq_types.OsqueryClient, d *dynamodb.DynamoDB, mut sync.Mutex) error {
	logger.Warn("Upserting Client: %v", oc)
	mut.Lock()
	//fmt.Println(oc)
	av, err := dynamodbattribute.MarshalMap(oc)
	if err != nil {
		logger.Warn("Marshal failed")
		logger.Warn(err)
		return err
	}
	res, err := d.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String("osquery_clients"),
		Item:      av,
	})
	fmt.Println("upsert res")
	fmt.Println(res)
	if err != nil {
		logger.Error(err)
		return err
	}
	mut.Unlock()
	return nil
}

//SearchByHostIdentifier search for a substring of a hostid
func SearchByHostIdentifier(hid string, s *dynamodb.DynamoDB) ([]osq_types.OsqueryClient, error) {
	Results := []osq_types.OsqueryClient{}
	type BS struct {
		HostIdentifier string `json:"host_identifier"`
	}
	var bs BS
	bs.HostIdentifier = hid
	scanItem := dynamodb.ScanInput{
		TableName: aws.String("osquery_clients"),
	}
	a, err := s.Scan(&scanItem)
	if err != nil {
		logger.Error(err)
		return Results, err
	}
	if len(hid) > 0 {
		for _, i := range a.Items {
			//fmt.Println(i)
			o := osq_types.OsqueryClient{}
			err := dynamodbattribute.UnmarshalMap(i, &o)
			if err != nil {
				logger.Error(err)
				return Results, err
			}
			if hid == string(o.Host_identifier) {
				Results = append(Results, o)
				fmt.Println(o)
			}

		}
	} else {
		for _, i := range a.Items {
			client := osq_types.OsqueryClient{}
			err = dynamodbattribute.UnmarshalMap(i, &client)
			if err != nil {
				logger.Error(err)
				return Results, err
			}
			Results = append(Results, client)
		}
	}
	//resp, err := s.GetItem(&item)
	if err != nil {
		logger.Error(err)
		return Results, err
	}
	return Results, nil
}

//ApprovePendingNode Approves pending node via nodeKey
func ApprovePendingNode(nodeKey string, dyn *dynamodb.DynamoDB, mut sync.Mutex) error {
	//approve a pending node validation.  Returns true if successfull
	osqNode, err := SearchByNodeKey(nodeKey, dyn)
	logger.Warn("here's our node that we're approving: %+v", osqNode)
	if err != nil {
		logger.Error(err)
		return err
	}
	if osqNode.Pending_registration_approval {
		logger.Info("[++] Approving Node")
		logger.Info(osqNode)
		newClient := osq_types.OsqueryClient{}
		newClient.Host_identifier = osqNode.Host_identifier
		newClient.Config_name = osqNode.Config_name
		newClient.Node_key = osqNode.Node_key
		newClient.Node_invalid = false
		newClient.HostDetails = osqNode.HostDetails
		newClient.Configuration_group = osqNode.Configuration_group
		newClient.Tags = osqNode.Tags
		err := UpsertClient(newClient, dyn, mut)
		if err != nil {
			logger.Error(err)
			return err
		}
	}
	return nil
}

//ValidNode returns if a node is valid or note, specified by nodeKey
func ValidNode(nodeKey string, dyn *dynamodb.DynamoDB) (bool, error) {
	osqNode, err := SearchByNodeKey(nodeKey, dyn)
	if err != nil {
		logger.Error(err)
		return false, err
	}
	if len(osqNode.Node_key) > 0 {
		if osqNode.Pending_registration_approval == false {
			return true, nil
		}
	}
	return false, nil
}

//SearchByNodeKey return osqueryClient by nodeKey
func SearchByNodeKey(nk string, s *dynamodb.DynamoDB) (osq_types.OsqueryClient, error) {

	type QS struct {
		NodeKey string `json:"node_key"`
	}
	var qs QS
	qs.NodeKey = nk
	osqNode := osq_types.OsqueryClient{}
	if len(nk) == 0 {
		return osqNode, errors.New("invalid node key")
	}
	js, err := dynamodbattribute.MarshalMap(qs)
	if err != nil {
		logger.Error(err)
		return osqNode, err
	}
	item := dynamodb.GetItemInput{
		TableName: aws.String("osquery_clients"),
		Key:       js,
	}
	resp, err := s.GetItem(&item)
	if err != nil {
		//panic(fmt.Sprintln(err, os.Stdout))
		log.Panic(err)
		return osqNode, err
	}
	if len(resp.Item) > 0 {
		err = dynamodbattribute.UnmarshalMap(resp.Item, &osqNode)
		if err != nil {
			fmt.Println(err)
			return osqNode, err
		}
		return osqNode, nil
	}
	return osqNode, nil
}

//GetPackQueries returns slice of packQueries
func APIGetPackQueries(dynamoDB *dynamodb.DynamoDB) ([]osq_types.PackQuery, error) {
	results := []osq_types.PackQuery{}
	scanItems, err := dynamoDB.Scan(&dynamodb.ScanInput{
		TableName: aws.String("osquery_packqueries"),
	})
	if err != nil {
		logger.Error(err)
		return results, err
	}
	for _, i := range scanItems.Items {
		packquery := osq_types.PackQuery{}
		err = dynamodbattribute.UnmarshalMap(i, &packquery)
		if err != nil {
			logger.Error(err)
			return results, err
		}
		results = append(results, packquery)
	}
	return results, nil

}

//APISearchPackQueries returns slice of packQueries which match the searchString substring
func APISearchPackQueries(searchString string, dynamoDB *dynamodb.DynamoDB) ([]osq_types.PackQuery, error) {
	results := []osq_types.PackQuery{}
	scanItems, err := dynamoDB.Scan(&dynamodb.ScanInput{
		TableName: aws.String("osquery_packqueries"),
	})
	if err != nil {
		logger.Error(err)
		return results, err
	}
	for _, i := range scanItems.Items {
		packquery := osq_types.PackQuery{}
		err = dynamodbattribute.UnmarshalMap(i, &packquery)
		if err != nil {
			logger.Error(err)
			return results, err
		}
		if strings.Contains(packquery.QueryName, searchString) {
			results = append(results, packquery)
		}
	}
	return results, nil

}

//GetPackQuery returns PackQuery by queryName
func GetPackQuery(queryName string, db *dynamodb.DynamoDB) (osq_types.PackQuery, error) {
	type QS struct {
		QueryName string `json:"query_name"`
	}
	qs := QS{}
	qs.QueryName = queryName
	packQuery := osq_types.PackQuery{}
	js, err := dynamodbattribute.MarshalMap(qs)
	if err != nil {
		logger.Error(err)
	}
	item := dynamodb.GetItemInput{
		TableName: aws.String("osquery_packqueries"),
		Key:       js,
	}
	resp, err := db.GetItem(&item)
	if err != nil {
		//panic(fmt.Sprintln(err, os.Stdout))
		log.Panic(err)
		return packQuery, err
	}
	if len(resp.Item) > 0 {
		err = dynamodbattribute.UnmarshalMap(resp.Item, &packQuery)
		if err != nil {
			fmt.Println(err)
			return packQuery, err
		}
		return packQuery, nil
	}
	return packQuery, nil
}

//UpsertPackQuery upsert pack query
func UpsertPackQuery(pq osq_types.PackQuery, db *dynamodb.DynamoDB, mut sync.Mutex) (bool, error) {
	mut.Lock()
	//fmt.Println(oc)
	av, err := dynamodbattribute.MarshalMap(pq)
	if err != nil {
		fmt.Println("marshal failed")
		fmt.Println(err)
	}
	_, err = db.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String("osquery_packqueries"),
		Item:      av,
	})
	if err != nil {
		fmt.Println(err)
		return false, err
	}
	mut.Unlock()
	return true, nil
}

//GetPackByName returns pack specified by name
func GetPackByName(s string, db *dynamodb.DynamoDB) (string, error) {
	type qs struct {
		PackName string `json:"packName"`
		PackOS   string `json:"pack_os"`
	}
	query := qs{}
	type pack struct {
		PackName string `json:"packName"`
		PackOS   string `json:"pack_os"`
		Queries   string `json:"queries"`
	}
	p := pack{}
	query.PackName = s
	query.PackOS = "Linux"
	js, err := dynamodbattribute.MarshalMap(query)
	item := dynamodb.GetItemInput{
		TableName: aws.String("osquery_packs"),
		Key:       js,
	}
	resp, err := db.GetItem(&item)
	if err != nil {
		logger.Info(err)
	}
	if len(resp.Item) > 0 {
		err = dynamodbattribute.UnmarshalMap(resp.Item, &p)
		if err != nil {
			logger.Info(err)
		}
		return p.Queries, nil
	}
	return p.Queries, nil
}

//GetNewPackByName returns a packs specified by packName
func GetNewPackByName(packName string, dynamoDB *dynamodb.DynamoDB) (osq_types.Pack, error) {
	pack := osq_types.Pack{}
	//create query string from pack name
	type QS struct {
		PackName string `json:"packName"`
	}
	queryString := QS{}
	queryString.PackName = packName
	//packQuery := PackQuery{}
	//qp := QueryPack{}
	//map queryString to attribute_map
	js, err := dynamodbattribute.MarshalMap(queryString)
	if err != nil {
	}
	//get pack map from dynamo
	resp, err := dynamoDB.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String("osquery_querypacks"),
		Key:       js,
	})
	//create empty pack to marshal data into
	type QueryPack struct {
		PackName string   `json:"packName"`
		Queries  []string `json:"queries"`
	}
	querypack := QueryPack{}
	if err != nil {
		//panic(fmt.Sprintln(err, os.Stdout))
		log.Panic(err)
		return pack, err
	}
	if len(resp.Item) > 0 {
		err = dynamodbattribute.UnmarshalMap(resp.Item, &querypack)
		if err != nil {
			fmt.Println(err)
			return pack, err
		}
		//here we actually build our osquery.Pack
		pack.PackName = querypack.PackName
		//pack.Queries = qp.Queries
		//itterate over list of queries and retrieve actual queries
		for _, query := range querypack.Queries {
			packquery, err := GetPackQuery(query, dynamoDB)
			if err != nil {
				logger.Error(err)
			}
			pack.Queries = append(pack.Queries, packquery)
		}
		return pack, nil
	}
	return pack, nil
}

//SearchQueryPacks returns a slice of QueryPacks
func SearchQueryPacks(searchString string, dynamoDB *dynamodb.DynamoDB) ([]osq_types.QueryPack, error) {
	results := []osq_types.QueryPack{}
	scanItems, err := dynamoDB.Scan(&dynamodb.ScanInput{
		TableName: aws.String("osquery_querypacks"),
	})
	if err != nil {
		logger.Error(err)
		return results, err
	}
	for _, i := range scanItems.Items {
		querypack := osq_types.QueryPack{}
		err = dynamodbattribute.UnmarshalMap(i, &querypack)
		if err != nil {
			logger.Error(err)
			return results, err
		}
		if strings.Contains(querypack.PackName, searchString) {
			results = append(results, querypack)
		}
	}
	return results, nil
}

//NewQueryPack creates a new query pack
func NewQueryPack(qp osq_types.QueryPack, dynamoDB *dynamodb.DynamoDB, mu sync.Mutex) error {
	av, err := dynamodbattribute.MarshalMap(qp)
	if err != nil {
		logger.Error(err)
		return err
	}
	mu.Lock()
	_, err = dynamoDB.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String("osquery_querypacks"),
		Item:      av,
	})
	mu.Unlock()
	if err != nil {
		logger.Error(err)
		return err
	}
	return nil
}

//DeleteQueryPack deletes specified query pack
func DeleteQueryPack(queryPackName string, dynamoDB *dynamodb.DynamoDB, mu sync.Mutex) error {
	type qs struct {
		PackName string `json:"pack_name"`
	}
	querystring := qs{queryPackName}
	av, err := dynamodbattribute.MarshalMap(querystring)
	if err != nil {
		logger.Error(err)
		return err
	}
	mu.Lock()
	_, err = dynamoDB.DeleteItem(&dynamodb.DeleteItemInput{
		TableName: aws.String("osquery_querypacks"),
		Key:       av,
	})
	mu.Unlock()
	if err != nil {
		logger.Error(err)
		return err
	}
	return nil
}

//UpsertPack upserts pack
func UpsertPack(qp osq_types.QueryPack, dynamoDB *dynamodb.DynamoDB, mu sync.Mutex) error {
	//Additive upsert.
	existing, err := GetNewPackByName(qp.PackName, dynamoDB)
	if err != nil {
		logger.Error(err)
		return err
	}
	switch len(existing.PackName) > 0 {
	case true:
		{
			existingQueries := map[string]bool{}
			//
			for _, packQuery := range existing.Queries {
				existingQueries[packQuery.QueryName] = true
			}
			if err != nil {
				logger.Error(err)
				return err
			}
			//note:  qp.Queries is a list of strings, not pack_queries
			for _, query := range qp.Queries {
				if !existingQueries[query] {
					//existing.Queries = append(existing.Queries, query)
					existingQueries[query] = true
				}
			}
			//existing queries should now be a map of both old and new
			logger.Debug(existingQueries)
			newQueryPack := osq_types.QueryPack{}
			newQueryPack.PackName = existing.PackName
			for query := range existingQueries {
				newQueryPack.Queries = append(newQueryPack.Queries, query)
			}
			err = DeleteQueryPack(qp.PackName, dynamoDB, mu)
			if err != nil {
				logger.Error(err)
				return err
			}
			err = NewQueryPack(newQueryPack, dynamoDB, mu)
			if err != nil {
				logger.Error(err)
				return err
			}
		}
	default:
		{
			err = NewQueryPack(qp, dynamoDB, mu)
			if err != nil {
				logger.Error(err)
				return err
			}
		}

	}

	return nil
}

//SearchDistributedNodeKey returns a distributed query for node specified by nodeKey
func SearchDistributedNodeKey(nk string, dynamoDB *dynamodb.DynamoDB) (osq_types.DistributedQuery, error) {
	type nodequery struct {
		NodeKey string `json:"node_key"`
	}
	querystring := nodequery{nk}
	marshalmap, err := dynamodbattribute.MarshalMap(querystring)
	dq := osq_types.DistributedQuery{}
	if err != nil {
		logger.Error(err)
		return dq, err
	}
	item := dynamodb.GetItemInput{
		TableName: aws.String("osquery_distributed_queries"),
		Key:       marshalmap,
	}
	resp, err := dynamoDB.GetItem(&item)
	if err != nil {
		logger.Error(err)
		return dq, err
	}
	if len(resp.Item) > 0 {
		err = dynamodbattribute.UnmarshalMap(resp.Item, &dq)
		if err != nil {
			logger.Warn("Error unmarshalling distributed query")
			logger.Error(err)
			return dq, err
		}

	}
	return dq, nil
}

//NewDistributedQuery creates new distirbuted query
func NewDistributedQuery(dq osq_types.DistributedQuery, dynamoDB *dynamodb.DynamoDB, mu sync.Mutex) error {
	mm, err := dynamodbattribute.MarshalMap(dq)
	if err != nil {
		logger.Error(err)
		return err
	}
	mu.Lock()
	_, err = dynamoDB.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String("osquery_distributed_queries"),
		Item:      mm,
	})
	if err != nil {
		logger.Error(err)
		return err
	}
	mu.Unlock()
	return nil
}

//DeleteDistributedQuery deletes specified distributed query
func DeleteDistributedQuery(dq osq_types.DistributedQuery, dynamoDB *dynamodb.DynamoDB, mu sync.Mutex) error {
	type querykey struct {
		NodeKey string `json:"node_key"`
	}
	var qk querykey
	qk.NodeKey = dq.NodeKey
	key, err := dynamodbattribute.MarshalMap(qk)
	if err != nil {
		logger.Error(err)
	}
	mu.Lock()
	_, err = dynamoDB.DeleteItem(&dynamodb.DeleteItemInput{
		TableName: aws.String("osquery_distributed_queries"),
		Key:       key,
	})
	if err != nil {
		logger.Error(err)
		return err
	}
	mu.Unlock()
	return nil
}

//AppendDistributedQuery appends a new distributed query to a nodes list of distributed queries
func AppendDistributedQuery(dq osq_types.DistributedQuery, dynamoDB *dynamodb.DynamoDB, mu sync.Mutex) error {
	//
	//NOTE:  This could be optimized to take in teh results of the already made call to check if the key exists
	// This is probably worth doing at some point when its beyond POC
	//should only be called if a check has been run to verify that the node_key for the
	//distributed query already exists (EG call distributed.SearchNodeKey())
	existingDQ, err := SearchDistributedNodeKey(dq.NodeKey, dynamoDB)
	//create copy of queries in existing distributed query
	existingQueries := map[string]bool{}
	for _, j := range existingDQ.Queries {
		existingQueries[j] = true
	}
	//delete existing distributed query
	err = DeleteDistributedQuery(existingDQ, dynamoDB, mu)
	if err != nil {
		logger.Error(err)
		return err
	}
	//append new queries to existing queries
	for _, j := range dq.Queries {
		if !existingQueries[j] {
			existingDQ.Queries = append(existingDQ.Queries, j)
		}
	}
	if err != nil {
		logger.Error(err)
		return err
	}
	//recreate distributed query with new + old queries
	err = NewDistributedQuery(existingDQ, dynamoDB, mu)
	if err != nil {
		logger.Error(err)
		return err
	}
	return nil
}

//UpsertDistributedQuery upserts a new distrubted query
func UpsertDistributedQuery(dq osq_types.DistributedQuery, dynamoDB *dynamodb.DynamoDB, mu sync.Mutex) error {
	//queries for node_key in dynamodb.  if found, appends queries to existing list
	//if not found, creates item and adds queries
	//Search for key
	//dynamoDB := dyndb.DbInstance()
	existing, err := SearchDistributedNodeKey(dq.NodeKey, dynamoDB)
	if err != nil {
		logger.Error(err)
		return err
	}
	switch len(existing.NodeKey) > 0 {
	case true:
		{
			err = AppendDistributedQuery(dq, dynamoDB, mu)
		}
	default:
		err = NewDistributedQuery(dq, dynamoDB, mu)
	}
	if err != nil {
		logger.Error(err)
		return err
	}
	return nil
}

//NewUser creates new user in DB
func NewUser(u osq_types.User, dynamoDB *dynamodb.DynamoDB, mu sync.Mutex) error {
	mm, err := dynamodbattribute.MarshalMap(u)
	if err != nil {
		logger.Error(err)
		return err
	}
	mu.Lock()
	_, err = dynamoDB.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String("osquery_users"),
		Item:      mm,
	})
	if err != nil {
		logger.Error(err)
		return err
	}
	mu.Unlock()
	return nil
}

//GetUser returns user from DB
func GetUser(username string, dynamoDB *dynamodb.DynamoDB) (osq_types.User, error) {
	u := osq_types.User{}
	type userquery struct {
		Username string `json:"username"`
	}

	userQuery := userquery{username}
	marshalmap, err := dynamodbattribute.MarshalMap(userQuery)
	if err != nil {
		logger.Error(err)
		return u, err
	}
	resp, err := dynamoDB.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String("osquery_users"),
		Key:       marshalmap,
	})
	if err != nil {
		logger.Info("get item failed")
		logger.Error(err)
		return u, err
	}
	err = dynamodbattribute.UnmarshalMap(resp.Item, &u)
	if err != nil {
		logger.Error(err)
		return u, err

	}
	return u, nil
}
