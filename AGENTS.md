## Deploy (поднять сборку)

При запросе «поднять сборку», «пересобрать», «запустить», «задеплоить»:
1. `docker stop vlesspanel && docker rm vlesspanel`
2. `docker compose -f /home/klem/VlessPanelWebApp/docker-compose.yml build vlesspanel`
3. `docker run -d --name vlesspanel --network vlesspanel-net -p 9090:8080 -v /home/klem/VlessPanelWebApp/data:/data -v /opt/aggregator-configs/configs:/opt/aggregator-configs/configs -e VLESSPANEL_PORT=8080 -e VLESSPANEL_AGGREGATOR_DIR=/opt/aggregator-configs/configs -e VLESSPANEL_PANELS_FILE=/data/panels.json -e VLESSPANEL_STATIC_DIR=/app/static -e VLESSPANEL_VLESSSUBTEST_DAEMON_URL=http://vlesssubtest:7070 --restart unless-stopped vlesspanelwebapp-vlesspanel:latest`

Примечание: `docker-compose` недоступен, используется `docker compose` (v2). Тег образа, собираемого compose, — `vlesspanelwebapp-vlesspanel:latest` (через дефис), как и в docker run; при расхождении берите фактический тег из вывода build.
