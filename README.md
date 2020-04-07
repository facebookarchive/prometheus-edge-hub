# Prometheus Edge Hub

Prometheus Edge Hub is a replacement of the Prometheus Pushgateway which allows for the pushing of metrics to an endpoint for scraping by prometheus, rather than having Prometheus scrape the metric sources directly. This differs from the Prometheus Pushgateway in several ways, the most important of which being that it does not overwrite timestamps and that metrics do not persist until updated. When the hub is scraped, all metrics are drained.

## Usage

Setup Prometheus to scrape the hub just as you would any other scrape target.

Example configuration:
```
scrape_configs:
  - job_name: 'prometheus'
    static_configs:
      - targets: ['localhost:9090']

  - job_name: "edge-hub"
    honor_labels: true
    metric_relabel_configs:
      - regex: 'job'
        action: labeldrop
      - regex: 'instance'
        action: labeldrop
    static_configs:
      - targets: ['edge-hub:9091']'
```

## Pushing Metrics

Pushing metrics to be scraped is as simple as making a post request to the `/metrics` endpoint containing a body with the metrics in [Prometheus Text Exposition Format](https://prometheus.io/docs/instrumenting/exposition_formats/).

## Debugging

To see the current state of the hub, make a GET request to `/debug`. This will return stats about the hub, such as how many metrics are stored in it. Use `/debug?verbose` to also see all of the metrics in the format that Prometheus would receive when scraping. Making a request to `/debug` does not remove the metrics from the hub.

## Runtime Options
Customize how the edge hub is run with these command-line options.
```
Usage of ./cache.o:
  -limit int
        Limit the total metrics in the cache at one time. Will reject a push if cache is full. Default is -1 which is no limit. (default -1)
  -port string
        Port to listen for requests. Default is 9091 (default "9091")
  -scrapeTimeout int
        Timeout for scrape calls. Default is 10 (default 10)
```
## Third-Party Code Disclaimer
Prometheus Edge Hub contains dependencies which are not maintained by the maintainers of this project. Please read the disclaimer at THIRD_PARTY_CODE_DISCLAIMER.md.

## License

Prometheus Edge Hub is MIT License licensed, as found in the LICENSE file.
