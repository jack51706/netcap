/*
 * NETCAP - Traffic Analysis Framework
 * Copyright (c) 2017 Philipp Mieden <dreadl0ck [at] protonmail [dot] ch>
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/dreadl0ck/netcap/utils"

	"github.com/dreadl0ck/netcap"
	"github.com/dreadl0ck/netcap/types"
)

func printHeader() {
	netcap.PrintLogo()
	fmt.Println()
	fmt.Println("usage examples:")
	fmt.Println("	$ net.export -r dump.pcap")
	fmt.Println("	$ net.export -iface eth0 -promisc=false")
	fmt.Println("	$ net.export -r TCP.ncap.gz")
	fmt.Println("	$ net.export .")
	fmt.Println()
}

// usage prints the use
func printUsage() {
	printHeader()
	flag.PrintDefaults()
}

func exportDir(path string) {

	files, err := ioutil.ReadDir(path)
	if err != nil {
		log.Fatal("failed to read dir: ", err)
	}

	var (
		wg    sync.WaitGroup
		count = 0
		times = map[string]time.Time{}
	)

	for _, f := range files {

		var (
			fName = f.Name()
			ext   = filepath.Ext(fName)
		)

		if ext == ".ncap" || ext == ".gz" {
			if !*flagReplay {

				fmt.Println("exporting", fName)

				// add to waitgroup
				wg.Add(1)

				// increase counter
				count++

				go func() {
					exportFile(fName)
					wg.Done()
				}()

				continue
			}
			times[fName] = firstTimestamp(fName)
		}
	}

	if *flagReplay {
		// determine the first timestamp
		var (
			begin     = time.Now()
			beginPath string
		)
		for p, t := range times {
			if t.Before(begin) {
				begin = t
				beginPath = p
			}
		}
		fmt.Println("exporting", beginPath)
		wg.Add(1)
		count++

		// time when we started to export the first file
		beginExportFirstFile := time.Now()

		// start exporting
		exportFile(beginPath)

		// remove this one from the map
		delete(times, beginPath)

		for p, t := range times {

			var (
				// copy to avoid capturing loop variable
				path = p

				// calculate delta to first timestamp
				deltaToBegin = t.Sub(begin)
			)

			fmt.Println("exporting", p, "in", deltaToBegin)

			// add to waitgroup
			wg.Add(1)

			// increase counter
			count++

			go func() {

				fmt.Println("SINCE:", time.Since(beginExportFirstFile))

				// sub the time needed to spawn the goroutine
				// from the previously calculated delta
				// usually around 1-3ms
				sleep := deltaToBegin - time.Since(beginExportFirstFile)

				// fmt.Println(p, sleep)

				// now sleep for the calculated interval
				// before starting to export the file
				time.Sleep(sleep)

				// begin exporting the file
				exportFile(path)

				// done
				wg.Done()
			}()
		}
	}

	fmt.Println("waiting for exports...")
	wg.Wait()

	fmt.Println("all exports finished!")
}

// this will open the netcap dump file at path
// and return the timestamp of the first audit record in there
func firstTimestamp(path string) time.Time {

	r, err := netcap.Open(path, netcap.DefaultBufferSize)
	if err != nil {
		log.Fatal("failed to open netcap file:", err)
	}
	defer r.Close()

	var (
		// read netcap file header
		header = r.ReadHeader()

		// initalize a record instance for the type from the header
		record = netcap.InitRecord(header.Type)
	)

	for {
		// read next record
		err := r.Next(record)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			// bail out on end of file
			break
		} else if err != nil {
			panic(err)
		}

		// assert to AuditRecord
		if p, ok := record.(types.AuditRecord); ok {
			return utils.StringToTime(p.Time())
		}
	}

	return time.Time{}
}

func exportFile(path string) {

	var (
		count  = 0
		r, err = netcap.Open(path, *flagMemBufferSize)
	)
	if err != nil {
		log.Fatal("failed to open netcap file:", err)
	}
	defer r.Close()

	var (
		// read netcap file header
		header = r.ReadHeader()

		// initalize a record instance for the type from the header
		record = netcap.InitRecord(header.Type)

		firstTimestamp time.Time
	)

	for {
		// read next record
		err := r.Next(record)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			// bail out on end of file
			break
		} else if err != nil {
			panic(err)
		}
		count++

		// assert to AuditRecord
		if p, ok := record.(types.AuditRecord); ok {

			if *flagReplay {
				t := utils.StringToTime(p.Time())
				if count == 1 {
					firstTimestamp = t
				} else {
					go func() {
						sleep := t.Sub(firstTimestamp)

						// fmt.Println(sleep)

						time.Sleep(sleep)
						// increment metric
						p.Inc()
					}()
					continue
				}
			}

			if *flagDumpJSON {
				// dump as JSON
				j, err := p.JSON()
				if err != nil {
					log.Fatal(err)
				}
				fmt.Println(j)
			}

			// increment metric
			p.Inc()
		} else {
			fmt.Printf("type: %#v\n", record)
			log.Fatal("type does not implement the types.AuditRecord interface")
		}
	}

	fmt.Println(path, "done. processed", count, "records.")
}
