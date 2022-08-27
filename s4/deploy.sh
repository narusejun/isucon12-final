#!/bin/bash -eux

# 各種設定ファイルのコピー
sudo cp -f env.sh /home/isucon/env.sh
#sudo cp -f etc/mysql/mariadb.conf.d/50-server.cnf /etc/mysql/mariadb.conf.d/50-server.cnf
#sudo cp -f etc/nginx/nginx.conf /etc/nginx/nginx.conf
#sudo cp -f etc/nginx/sites-available/isucondition.conf /etc/nginx/sites-available/isucondition.conf
#sudo nginx -t

# アプリケーションのビルド
#cd /home/isucon/webapp/go
#make

# ミドルウェア・Appの再起動
#sudo systemctl restart mariadb
#sudo systemctl reload nginx
#sudo systemctl restart isuxxxx
