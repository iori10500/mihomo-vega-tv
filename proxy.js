export function makeConfig(url) {
  if (!/^https:\/\/[^\s/]+(?:\/[^\s]*)?$/i.test(url)) throw Error('请输入完整 HTTPS 订阅地址');
  return JSON.stringify({
    'log-level':'silent', mode:'rule',
    'proxy-providers':{nodes:{type:'http',url,path:'./nodes-' + Date.now() + '.dat',interval:21600}},
    'proxy-groups':[{name:'TV',type:'url-test',use:['nodes'],url:'https://www.gstatic.com/generate_204',interval:300}],
    rules:['MATCH,TV'],
  });
}
