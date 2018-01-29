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
import "fmt"
import "strconv"
import "time"
import "strings"

func exitErrorf(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
	os.Exit(1)
}

func parseJson(v []byte) interface{} {
	var f interface{}
	if err := json.Unmarshal(v, &f); err != nil {
		return nil
	} else {
		return f
	}
}

func toInt(v string, defValue int, errValue int) int {
	if len(v) == 0 {
		return defValue
	}
	v2, err := strconv.Atoi(v)
	if err != nil {
		return errValue
	}
	return v2
}

func toFloat(v string, defValue float64, errValue float64) float64 {
	if len(v) == 0 {
		return defValue
	}
	v2, err := strconv.ParseFloat(v, 32)
	if err != nil {
		return errValue
	}
	return v2

}

func toBool(v string, defValue bool) bool {
	if len(v) == 0 {
		return defValue
	} else {
		return "true" == strings.ToLower(v)
	}
}

func main() {

	region := os.Getenv("AWS_REGION")
	bucketName := os.Getenv("AWS_BUCKET")
	mapSize := os.Getenv("CMDB_MAP_SIZE")
	dbName := os.Getenv("CMDB_NAME")
	archiveName := fmt.Sprintf("%s.zip", dbName)
	port := os.Getenv("CMDB_PORT")
	dataDir := os.Getenv("CMDB_DATA")

	dbFiles := []string{
		fmt.Sprintf("%s/%s", dataDir, "data.mdb"),
		fmt.Sprintf("%s/%s", dataDir, "lock.mdb"),
	}

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region)})
	svc := s3.New(sess)

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
	err = env.Open(dataDir, 0, 0644)
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

	app.Get("/{key:string}", func(ctx iris.Context) {
		key := ctx.Params().Get("key")
		skip := toInt(ctx.FormValue("skip"), 0, -1)
		count := toInt(ctx.FormValue("count"), 0, -1)

		if skip == -1 || count == -1 {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}

		err := env.View(func(txn *lmdb.Txn) (err error) {
			if count > 0 {
				cur, err := txn.OpenCursor(dbi)
				if err != nil {
					return err
				}
				_, v, err := cur.Get([]byte(key), nil, lmdb.Set)
				if err != nil {
					return err
				}

				ctx.Write([]byte("["))

				firstWritten := false

				if skip == 0 {
					ctx.Write(v)
					firstWritten = true
				}

				read := 1
				for {

					if read == count {
						break
					}
					_, v, err = cur.Get(nil, nil, lmdb.NextDup)
					if lmdb.IsNotFound(err) {
						break
					} else if err != nil {
						return err
					} else {
						if read >= skip {
							if firstWritten == true {
								ctx.Write([]byte(","))
							} else {
								firstWritten = true
							}
							ctx.Write(v)

						}
					}

					read++
				}

				ctx.Write([]byte("]"))
				ctx.ContentType("application/json")

			} else {
				v, err := txn.Get(dbi, []byte(key))
				if err != nil {
					return err
				}
				ctx.ContentType("application/json")
				ctx.Write(v)
			}

			return nil
		})

		if err != nil {
			if lmdb.IsNotFound(err) {
				ctx.StatusCode(iris.StatusNotFound)
			} else {
				log.Fatal("error: %s", err)
				ctx.StatusCode(iris.StatusInternalServerError)
			}
		}
	})
	app.Post("/{key:string}", func(ctx iris.Context) {
		key := ctx.Params().Get("key")
		body, _ := ioutil.ReadAll(ctx.Request().Body)
		if parseJson(body) == nil {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}

		unique := toBool(ctx.FormValue("unique"), false)

		err := env.Update(func(txn *lmdb.Txn) (err error) {
			if unique {
				err := txn.Put(dbi, []byte(key), body, lmdb.NoOverwrite)
				if err != nil {
					log.Printf("error: %s", err)
					ctx.StatusCode(iris.StatusConflict)
					return nil
				} else {
					return err
				}
			} else {
				return txn.Put(dbi, []byte(key), body, 0)
			}
		})

		if err != nil {
			log.Fatal("Can't write: %s", err)
			ctx.StatusCode(iris.StatusInternalServerError)
		}
	})

	type Place struct {
		City    string
		Country string
		Lat     float32
		Lon     float32
	}

	type Coords struct {
		Lat float32
		Lon float32
	}

	type LoadResult struct {
		read    int
		written int
	}

	app.Post("/backups/new", func(ctx iris.Context) {
		t := time.Now()
		tstamp := fmt.Sprintf("%d%02d%02d%02d%02d%02d",
			t.Year(), t.Month(), t.Day(),
			t.Hour(), t.Minute(), tNoOverwrite)

		remoteFileName := fmt.Sprintf("%s-%s", tstamp, archiveName)
		localFileName := fmt.Sprintf("%s/%s-%s", dataDir, tstamp, archiveName)
		err := archiver.Zip.Make(localFileName, dbFiles)
		if err != nil {
			log.Printf("Could not create archive: %s", err)
			return
		}
		file, err := os.Open(localFileName)
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
			Key:    aws.String(remoteFileName),
			Body:   file,
		})
		if err != nil {
			log.Printf("Unable to upload %q to %q, %v", localFileName, bucketName, err)
		}

		ctx.JSON(iris.Map{"name": tstamp, "size": fi.Size()})

	})
	app.Post("/backups/{key:string}/restore", func(ctx iris.Context) {
		key := ctx.Params().Get("key")
		localFileName := fmt.Sprintf("%s/%s-%s", dataDir, key, archiveName)
		remoteFileName := fmt.Sprintf("%s-%s", key, archiveName)
		file, err := os.Create(localFileName)
		defer file.Close()
		if err != nil {
			log.Fatal("Unable to open file for writing item %q, %v", archiveName, err)
			ctx.StatusCode(iris.StatusInternalServerError)
			return
		}

		downloader := s3manager.NewDownloader(sess)
		bytes, err := downloader.Download(file,
			&s3.GetObjectInput{
				Bucket: aws.String(bucketName),
				Key:    aws.String(remoteFileName),
			})
		if err != nil {
			log.Printf("Unable to download item %q, %v", remoteFileName, err)
			return
		}

		err = archiver.Zip.Open(localFileName, dataDir)
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
		read := 1
		written := 0
		for {
			line, err := reader.Read()
			if err == io.EOF {
				break
			} else if err != nil {
				log.Fatal("error reading line: %s", err)
			} else {

				lat := float32(toFloat(line[5], 0, 0))
				lon := float32(toFloat(line[6], 0, 0))
				coord := []float32{lat, lon}
				k, err := json.Marshal(coord)

				if err != nil {
					log.Print("error serializing json: %s", err)
				} else {
					kBytes := []byte(k)
					tokens := strings.Split(line[1], " ")
					v := Place{line[2], line[0], lat, lon}
					place, err := json.Marshal(v)
					if err != nil {
						log.Print("error serializing json: %s", err)
					} else {
						err := env.Update(func(txn *lmdb.Txn) (err error) {

							err = txn.Put(dbi, kBytes, place, 0)
							if err != nil {
								return err
							} else {
								//log.Printf("Written %s => %s", kBytes, place)
								written++
								for _, t := range tokens {
									tokens2 := strings.Split(t, "-")
									for _, t2 := range tokens2 {
										if len(t2) > 0 {
											err = txn.Put(dbi, []byte(t2), kBytes, 0)
											if err != nil {
												log.Printf("Error while writing %s", t2)
											} else {
												written++
												if written%1000 == 0 {
													log.Printf("Written %s keys", written)
												}

												//log.Printf("Written %s => %s", t2, kBytes)
											}
										} else {
											//log.Print("Skipping empty key")
										}
									}
								}
								return nil
							}

						})

						if err != nil {
							log.Printf("error writing: %s", err)
						}

					}
				}
			}

			read++
		}

		ctx.ContentType("application/json")
		ctx.JSON(iris.Map{"read": read, "written": written})
	})

	app.Run(iris.Addr(fmt.Sprintf(":%s", port)))
}
