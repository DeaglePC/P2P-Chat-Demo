# 简介
基于P2P的聊天程序，通过UDP打洞实现，为了学习P2P技术建立的项目

# 原理
通过P2P服务器获取对方当前公网IP:Port，通过这个地址进行打洞

# 安装
```shell
git clone https://github.com/DeaglePC/P2P-Chat-Demo.git && cd P2P-Chat-Demo && make && cd bin && ls
```

# 使用
1. 检测NAT类型
```shell
python3 -c "$(curl -fsSL https://raw.githubusercontent.com/evilpan/P2P-Over-MiddleBoxes-Demo/master/stun/classic_stun_client.py)"
```
**如果是对称型则无法使用**

2. 在有公网IP的机器运行`p2pserver`
```shell
./p2pserver -port 11223
```

3. 在两个不同的NAT下运行`p2pclient`
```
./p2pclient -raddr ip:port -laddr 0.0.0.0:port
#login name
#get ID
#punch ID
ID msg
```
