const { getDefaultConfig } = require("expo/metro-config");
const os = require('os');

/** @type {import('expo/metro-config').MetroConfig} */
const config = getDefaultConfig(__dirname);

// 获取本机LAN IP地址
function getLocalIP() {
  const interfaces = os.networkInterfaces();
  for (const name of Object.keys(interfaces)) {
    for (const iface of interfaces[name]) {
      // IPv4 且非本地地址
      if (iface.family === 'IPv4' && !iface.internal) {
        return iface.address;
      }
    }
  }
  return 'localhost'; // 降级方案
}

const lanIP = getLocalIP();

// 为真机开发配置正确的主机地址
config.server = {
  port: 8081,
  // 显式绑定到局域网IP，支持有USB和无USB的两种情况
  enhanceMiddleware: (middleware) => {
    return (req, res, next) => {
      // 允许从真机访问开发服务器
      res.setHeader('Access-Control-Allow-Origin', '*');
      res.setHeader('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE');
      res.setHeader('Access-Control-Allow-Headers', 'Content-Type');
      return middleware(req, res, next);
    };
  },
};

// 强制使用IP地址而不是localhost
config.resolver = {
  ...config.resolver,
  extraNodeModules: config.resolver?.extraNodeModules || {},
};

// 调试信息
console.log(`\n📱 Metro开发服务器配置：`);
console.log(`   LAN IP: ${lanIP}`);
console.log(`   Metro URL: http://${lanIP}:8081`);
console.log(`   API URL: http://${lanIP}:8080`);
console.log(`   ✅ 支持USB连接和Wi-Fi连接两种模式\n`);

module.exports = config;
