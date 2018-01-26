package main

import "github.com/aws/aws-sdk-go/aws"
import "github.com/aws/aws-sdk-go/aws/session"
import "github.com/aws/aws-sdk-go/service/s3"
import "github.com/aws/aws-sdk-go/service/s3/s3manager"
import "github.com/kataras/iris"
import "github.com/mholt/archiver"
import "io/ioutil"
import "github.com/bmatsuo/lmdb-go/lmdb"
import "log"
import "os"
import "encoding/csv"
import "encoding/json"
import "bufio"
import "io"
import "strings"
import "fmt"
import "strconv"
import "time"

func exitErrorf(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
	os.Exit(1)
}

func main() {

	region := os.Getenv("AWS_REGION")
	bucketName := os.Getenv("AWS_BUCKET")
	mapSize := os.Getenv("CMDB_MAP_SIZE")
	dbName := os.Getenv("CMDB_NAME")
	archiveName := fmt.Sprintf("%s.zip", dbName)
	port := os.Getenv("CMDB_PORT")

	dbFiles := []string{
		"data.mdb",
		"lock.mdb",
	}

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region)})
	svc := s3.New(sess)

	resp, err := svc.ListObjects(&s3.ListObjectsInput{Bucket: aws.String(bucketName)})
	if err != nil {
		exitErrorf("Unable to list items in bucket %q, %v", "in-fullpass-backups", err)
	}

	for _, item := range resp.Contents {
		fmt.Println("Name:         ", *item.Key)
		fmt.Println("Last modified:", *item.LastModified)
		fmt.Println("Size:         ", *item.Size)
		fmt.Println("Storage class:", *item.StorageClass)
		fmt.Println("")
	}

	result, err := svc.ListBuckets(&s3.ListBucketsInput{})
	if err != nil {
		exitErrorf("Unable to list buckets, %v", err)
	}

	fmt.Println("Buckets:")

	for _, b := range result.Buckets {
		fmt.Printf("* %s created on %s\n",
			aws.StringValue(b.Name), aws.TimeValue(b.CreationDate))
	}

	env, err := lmdb.NewEnv()
	if err != nil {
		log.Printf("Could not create env: %s", err)
		return
	}
	defer env.Close()

	err = env.SetMaxDBs(1)
	if err != nil {
		log.Printf("Could not set max dbs: %s", err)
		return
	}

	mapSizeBytes, err := strconv.ParseInt(mapSize, 10, 64)
	if err != nil {
		exitErrorf("Invalid map size: %s", mapSize, err)
	}

	err = env.SetMapSize(mapSizeBytes)
	if err != nil {
		exitErrorf("Could not set map size: %s", err)
	}
	err = env.Open(".", 0, 0644)
	if err != nil {
		log.Printf("Could not open db: %s", err)
		return
	}

	staleReaders, err := env.ReaderCheck()
	if err != nil {
		log.Printf("Could not check for stale readers: %s", err)
		return
	}
	if staleReaders > 0 {
		log.Printf("cleared %d reader slots from dead processes", staleReaders)
	}
	var dbi lmdb.DBI
	err = env.Update(func(txn *lmdb.Txn) (err error) {
		dbi, err = txn.OpenDBI("default", lmdb.Create|lmdb.DupSort)
		return err
	})
	if err != nil {
		log.Printf("Could not open db: %s", err)
		return
	}

	app := iris.Default()

	app.Get("/keys/{key:string}", func(ctx iris.Context) {
		key := ctx.Params().Get("key")
		err = env.View(func(txn *lmdb.Txn) (err error) {
			v, err := txn.Get(dbi, []byte(key))
			if err != nil {
				return err
			}
			ctx.Write(v)
			return nil
		})

		if err != nil {
			log.Print("error: %s", err)
			ctx.Text("not_found")
		}
	})
	app.Post("/keys/{key:string}", func(ctx iris.Context) {
		key := ctx.Params().Get("key")
		body, _ := ioutil.ReadAll(ctx.Request().Body)
		err = env.Update(func(txn *lmdb.Txn) (err error) {
			err = txn.Put(dbi, []byte(key), body, 0)
			return err
		})
		if err != nil {
			log.Print("error: %s", err)
			ctx.Write(body)
		}
		ctx.Write([]byte("{}"))

	})

	type Place struct {
		City    string
		Country string
		Lat     string
		Lon     string
	}

	app.Post("/backups", func(ctx iris.Context) {
		t := time.Now()
		tstamp := fmt.Sprintf("%d%02d%02d%02d%02d%02d",
			t.Year(), t.Month(), t.Day(),
			t.Hour(), t.Minute(), t.Second())

		fileName := fmt.Sprintf("%s-%s", tstamp, archiveName)
		err := archiver.Zip.Make(archiveName, dbFiles)
		if err != nil {
			log.Printf("Could not create archive: %s", err)
			return
		}
		file, err := os.Open(archiveName)
		defer file.Close()
		if err != nil {
			log.Printf("Unable to open file %q, %v", err)
			return
		}

		fi, err := file.Stat()
		if err != nil {
			log.Printf("Unable to obtain file size: %s", err)
		}

		uploader := s3manager.NewUploader(sess)
		_, err = uploader.Upload(&s3manager.UploadInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(fileName),
			Body:   file,
		})
		if err != nil {
			log.Printf("Unable to upload %q to %q, %v", fileName, bucketName, err)
		}

		ctx.JSON(iris.Map{"name": tstamp, "size": fi.Size()})

	})
	app.Post("/restore/{key:string}", func(ctx iris.Context) {
		key := ctx.Params().Get("key")
		fileName := fmt.Sprintf("%s-%s", key, archiveName)

		file, err := os.Create(archiveName)
		if err != nil {
			log.Printf("Unable to open file for writing item %q, %v", archiveName, err)
			return
		}
		defer file.Close()

		downloader := s3manager.NewDownloader(sess)
		bytes, err := downloader.Download(file,
			&s3.GetObjectInput{
				Bucket: aws.String(bucketName),
				Key:    aws.String(fileName),
			})
		if err != nil {
			log.Printf("Unable to download item %q, %v", fileName, err)
			return
		}

		err = archiver.Zip.Open(archiveName, ".")
		if err != nil {
			log.Printf("Unable to unarchive %s, %v", archiveName, err)
			return
		}

		ctx.JSON(iris.Map{"size": bytes})
	})
	app.Post("/_load", func(ctx iris.Context) {
		csvFile, _ := os.Open("./locations_big.txt")
		reader := csv.NewReader(bufio.NewReader(csvFile))
		reader.LazyQuotes = true
		reader.Read()
		for {
			line, err := reader.Read()
			if err == io.EOF {
				break
			} else if err != nil {
				log.Fatal("error reading line: %s", err)
			} else {
				k := strings.Join([]string{line[5], line[6]}, "-")
				v := Place{line[2], line[2], line[5], line[6]}
				b, err := json.Marshal(v)
				if err != nil {
					log.Print("error serializing json: %s", err)
				} else {
					err := env.Update(func(txn *lmdb.Txn) (err error) {
						err = txn.Put(dbi, []byte(k), b, 0)
						return err
					})
					if err != nil {
						log.Printf("error writing: %s", err)
					} else {
						//log.Print("written: ", k)
					}
				}
			}
		}
	})

	app.Run(iris.Addr(fmt.Sprintf(":%s", port)))
}
