package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"InstaStellar/messagebus"
	consumer "InstaStellar/messagebus/consumer"

	"github.com/OWASP/Amass/amass"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const TimeOut = 7 * time.Second

type Person struct {
	Name   string
	Domain string
}

var mongoSession *mgo.Session

func initializeMongo() {
	session, err := mgo.Dial("localhost")
	mongoSession = session
	if err != nil {
		panic(err)
	}
	// defer mongoSession.Close()

	// Optional. Switch the session to a monotonic behavior.
	mongoSession.SetMode(mgo.Monotonic, true)

}

type MongoStruct struct {
	name   string `json:"name" bson:"name"`
	domain string `json:"domain" bson:"domain"`
}

const CollectionNames = "DOMAINS"

func IngestInMongo(dataToIngest Response) {

	c := mongoSession.DB("test").C(CollectionNames)

	upsertdata := bson.M{"$push": bson.M{"data": dataToIngest.dataSet}}
	_, err := c.Upsert(bson.M{"_id": dataToIngest.id}, upsertdata)
	if err != nil {
		log.Fatal(err)
	}

}

func MarkFinished(_id bson.ObjectId) {

	c := mongoSession.DB("test").C(CollectionNames)
	upsertdata := bson.M{"$set": bson.M{"status": "DONE"}}
	_, err := c.Upsert(bson.M{"_id": _id}, upsertdata)
	if err != nil {
		log.Fatal(err)
	}

}

func main() {
	initializeMongo()

	noOfWorkerPools := 1
	wg := sync.WaitGroup{}
	wg.Add(1)
	channel := make(chan Response)
	inChannel := make(chan messagebus.MessagePasser)

	for i := 0; i < noOfWorkerPools; i++ {
		go EnqueueDomain(inChannel, channel)
	}

	go consumer.Consume(inChannel)

	go func() {

		for {
			response := <-channel
			IngestInMongo(response)
		}
	}()

	wg.Wait()

}

type MessagePasser struct {
	Id     string
	Domain string
}

type Response struct {
	id      bson.ObjectId
	running bool
	dataSet amass.AmassOutput
	domain  string
}

func EnqueueDomain(inChannel <-chan messagebus.MessagePasser, response chan<- Response) {

	for domainName := range inChannel {
		ctx, _ := context.WithTimeout(context.Background(), TimeOut)
		fmt.Println("Target", domainName)

		enum := amass.NewEnumeration()
		enum.Passive = true
		// enum.Active = true

		go func() {

			for result := range enum.Output {

				response <- Response{
					bson.ObjectIdHex(domainName.Id),
					false,
					*result,
					domainName.Domain,
				}

			}

		}()

		// Seed the default pseudo-random number generator
		rand.Seed(time.Now().UTC().UnixNano())
		enum.AddDomain(domainName.Domain)
		enum.Start(ctx)
		// enum.Pause()
		MarkFinished(bson.ObjectIdHex(domainName.Id))
		fmt.Println("Done!", domainName)

	}
}
