package command

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/coreos/etcd/Godeps/_workspace/src/github.com/codegangsta/cli"
	"github.com/coreos/etcd/Godeps/_workspace/src/golang.org/x/net/context"
	"github.com/coreos/etcd/client"
)

func NewClusterHealthCommand() cli.Command {
	return cli.Command{
		Name:  "cluster-health",
		Usage: "check the health of the etcd cluster",
		Flags: []cli.Flag{
			cli.BoolFlag{Name: "forever", Usage: "forever check the health every 10 second until CTRL+C"},
		},
		Action: handleClusterHealth,
	}
}

func handleClusterHealth(c *cli.Context) {
	forever := c.Bool("forever")
	if forever {
		sigch := make(chan os.Signal, 1)
		signal.Notify(sigch, os.Interrupt)

		go func() {
			<-sigch
			os.Exit(0)
		}()
	}

	tr, err := getTransport(c)
	if err != nil {
		handleError(ExitServerError, err)
	}

	hc := http.Client{
		Transport: tr,
	}

	cln := mustNewClientNoSync(c)
	mi := client.NewMembersAPI(cln)
	ms, err := mi.List(context.TODO())
	if err != nil {
		fmt.Println("cluster may be unhealthy: failed to list members")
		handleError(ExitServerError, err)
	}

	for {
		health := false
		for _, m := range ms {
			if len(m.ClientURLs) == 0 {
				fmt.Printf("member %s is unreachable: no available published client urls\n", m.ID)
				continue
			}

			checked := false
			for _, url := range m.ClientURLs {
				resp, err := hc.Get(url + "/health")
				if err != nil {
					fmt.Printf("failed to check the health of member %s on %s: %v\n", m.ID, url, err)
					continue
				}

				result := struct{ Health string }{}
				nresult := struct{ Health bool }{}
				bytes, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					fmt.Printf("failed to check the health of member %s on %s: %v\n", m.ID, url, err)
					continue
				}
				resp.Body.Close()

				err = json.Unmarshal(bytes, &result)
				if err != nil {
					err = json.Unmarshal(bytes, &nresult)
				}
				if err != nil {
					fmt.Printf("failed to check the health of member %s on %s: %v\n", m.ID, url, err)
					continue
				}

				checked = true
				if result.Health == "true" {
					health = true
					fmt.Printf("member %s is healthy: got healthy result from %s\n", m.ID, url)
				} else {
					fmt.Printf("member %s is unhealthy: got unhealthy result from %s\n", m.ID, url)
				}
				break
			}
			if !checked {
				fmt.Printf("member %s is unreachable: %v are all unreachable\n", m.ID, m.ClientURLs)
			}
		}
		if health {
			fmt.Println("cluster is healthy")
		} else {
			fmt.Println("cluster is unhealthy")
		}

		if !forever {
			break
		}
		fmt.Printf("\nnext check after 10 second...\n\n")
		time.Sleep(10 * time.Second)
	}
}
