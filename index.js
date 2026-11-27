import React,{useCallback,useEffect,useRef,useState} from 'react';
import {AppRegistry,AppState,BackHandler,Linking,Pressable,Text,TextInput,View,useWindowDimensions} from 'react-native';
import {SecurityManager} from '@amazon-devices/security-manager-lib';
import {measureMemory} from './memory';
import Core from './src/NativeCore';
import {makeConfig} from './proxy';
import Management from './Management';
import {translator,diagnostic} from './i18n';
import {useLanguage} from './useLanguage';
import {name} from './app.json';

const ink='#101722',panel='#1a2635',line='#344557',white='#f1f6fa',dim='#a8b8c7',accent='#79dbff',green='#82e5b8';
const pause=ms=>new Promise(resolve=>setTimeout(resolve,ms));
const permission='com.amazon.network.privilege.net-vpn-ctrl';
const modeNames={smart:'智能分流',global:'全局代理'};
const services=['哔哩哔哩','腾讯视频','爱奇艺','优酷','芒果 TV'];
const choicesList=[['f','跟随智能规则'],['d','始终直连'],['p','始终代理']];
function observations(raw){
  if(!raw.startsWith('{'))return {};
  try{return JSON.parse(raw);}catch{return {};}
}
function routeName(route){return route==='direct'?'直连':route==='proxy'?'代理':'待观察';}
const stateNames={running:'已连接',stopped:'已断开',starting:'正在连接',recovering:'后台待恢复',orphaned:'后台已退出',unconfigured:'等待订阅'};
function Label({children,size,color=white,bold=false,style}) {
  return <Text style={[{color,fontSize:size,fontWeight:bold?'700':'400'},style]}>{children}</Text>;
}
function Tile({children,action,preferred,style,s,disabled=false}) {
  const [focused,setFocused]=useState(false);
  return <Pressable focusable hasTVPreferredFocus={preferred} onFocus={()=>setFocused(true)} onBlur={()=>setFocused(false)}
    onPress={disabled?undefined:action} style={[{borderWidth:3*s,borderRadius:18*s,backgroundColor:panel,borderColor:line},style,focused&&{borderColor:accent,backgroundColor:'#213a4c'}]}>
    {children}
  </Pressable>;
}
function Power({s,active}) {
  const c=active?green:accent;
  return <View style={{width:94*s,height:94*s,borderRadius:47*s,backgroundColor:'#172230',alignItems:'center',justifyContent:'center'}}>
    <View style={{width:49*s,height:49*s,borderRadius:25*s,borderWidth:5*s,borderColor:c}}/>
    <View style={{position:'absolute',top:15*s,width:16*s,height:28*s,backgroundColor:'#172230',alignItems:'center'}}>
      <View style={{backgroundColor:c,width:5*s,height:27*s,borderRadius:3*s}}/>
    </View>
  </View>;
}
function App(){
  const ui=useLanguage(), t=translator(ui.language);
  const {width,height}=useWindowDimensions(),s=Math.min(width/1600,height/900);
  const [state,setState]=useState(''),[mode,setMode]=useState('smart'),[report,setReport]=useState('');
  const [edit,setEdit]=useState(false),[url,setUrl]=useState(''),[note,setNote]=useState(''),[working,setWorking]=useState(false);
  const [servicePage,setServicePage]=useState(false),[choices,setChoices]=useState('fffff');
  useEffect(()=>measureMemory('ui'),[]);
  const [section,setSection]=useState('');
  const busy=useRef(false);
  const refresh=useCallback(()=>{
    try {setState(Core.coreState());setMode(Core.routingMode());setChoices(Core.serviceChoices());setReport(Core.routeReport());}
    catch(e){setNote(e.message);}
  },[]);
  useEffect(()=>{
    function resume() {
      refresh();
      const needsService=()=>Core.requested()?Core.coreState()!=='running':Core.coreState()==='orphaned';
      if(!needsService())return;
      SecurityManager.getPrivilegeState(permission).then(x=>{
        if(!needsService())return;
        if(x==='ALLOW')return Linking.openURL('pkg://com.iori.mihomo.service');
        if(Core.requested())setNote('请按确定键连接，并在电视提示中允许 VPN 连接');
      }).catch(e=>setNote(e.message));
    }
    resume();
    const listener=AppState.addEventListener('change',next=>{if(next==='active')resume();});
    const timer=setInterval(refresh,1000);
    return ()=>{clearInterval(timer);listener.remove();};
  },[refresh]);
  useEffect(()=>{if(state==='running'&&report)console.info('MihomoTV routing '+report);},[state,report]);
  function back(){if(section==='language'){setSection('');return true;}if(edit==='privacy'){setEdit(true);return true;}if(servicePage){setServicePage(false);setNote('');return true;}if(!edit)return false;if(!busy.current){setEdit(false);setUrl('');setNote('');}return true;}
  useEffect(()=>{const l=BackHandler.addEventListener('hardwareBackPress',back);return ()=>l.remove();},[edit,servicePage,section]);
  function request(value){const err=Core.setRequested(value);if(err)throw Error(err);}
  async function disconnect(){
    request(false);
    await Linking.openURL('pkg://com.iori.mihomo.service');
    for(let i=0;i<40;i++){
      const current=Core.coreState();
      if(current==='stopped')return;
      if(!['running','orphaned','starting'].includes(current))throw Error(stateNames[current]||current);
      await pause(200);
    }
    throw Error('后台尚未断开，请重试');
  }
  async function connect(){
    let grant=await SecurityManager.getPrivilegeState(permission);
    if(grant!=='ALLOW')grant=await SecurityManager.requestPrivilege(permission);
    if(grant!=='ALLOW')throw Error('请在电视提示中允许 VPN 连接');
    request(true);await Linking.openURL('pkg://com.iori.mihomo.service');
    for(let i=0;i<160;i++){
      const current=Core.coreState();if(current==='running')return;
      if(!['starting','stopped','recovering'].includes(current))throw Error(stateNames[current]||current);
      await pause(250);
    }
    throw Error('连接超时，请检查网络后重试');
  }
  async function act(fn){
    if(busy.current)return;busy.current=true;setWorking(true);setNote('');
    try{await fn();}catch(e){setNote(e.message);}finally{busy.current=false;setWorking(false);refresh();}
  }
  async function save(){
    const config=makeConfig(url.trim());
    if(Core.requested()||Core.coreState()==='running')await disconnect();
    const err=Core.saveConfig(config);if(err)throw Error(err);
    await connect();setUrl('');setEdit(false);
  }
  const active=state==='running',configured=state!=='unconfigured';
  const actual=observations(report);
  const evidence=active && actual.mode===mode && actual.choices===choices && !actual.error;
  const [api,cover,media]=actual.bili||[];
  const statusCaption=!active?t('VPN 未连接'):diagnostic(t,actual.error)||t(!evidence?'正在应用规则':'本次设置下的实际出站');
  const networkStatus=t(stateNames[state]|| (state?'连接异常':'正在读取'));
  const body={flex:1,backgroundColor:ink,paddingHorizontal:68*s,paddingTop:54*s};
  const row={flexDirection:'row',alignItems:'center'};
  function modeTile(value,description){
    const selected=mode===value;
    return <Tile s={s} style={{paddingHorizontal:26*s,paddingVertical:21*s,marginBottom:14*s}} action={()=>act(()=>{
      const err=Core.setRoutingMode(value);if(err)throw Error(err);
    })}><View style={row}><View style={{width:24*s,height:24*s,borderRadius:12*s,borderColor:selected?accent:dim,borderWidth:2*s,alignItems:'center',justifyContent:'center',marginRight:17*s}}>
      {selected&&<View style={{width:12*s,height:12*s,borderRadius:6*s,backgroundColor:accent}}/>}</View>
      <Label size={31*s} bold>{t(modeNames[value])}</Label></View>
      <Label size={23*s} color={dim} style={{marginTop:7*s,marginLeft:41*s}}>{t(description)}</Label></Tile>;
  }
  if(section==='language')return <View style={body}>
    <View style={[row,{justifyContent:'space-between',marginBottom:30*s}]}>
      <Label bold size={48*s}>{t('语言')}</Label>
      <Tile s={s} action={()=>setSection('')} style={{paddingHorizontal:25*s,paddingVertical:12*s}}><Label size={24*s}>{t('返回')}</Label></Tile>
    </View>
    {[['system',t('跟随电视语言')],['en','English'],['zh','简体中文']].map(([value,title])=>
      <Tile s={s} key={value} preferred={value===ui.preference} disabled={!ui.ready} action={()=>ui.select(value)}
        style={{padding:20*s,marginBottom:14*s}}>
        <Label size={30*s} color={value===ui.preference?green:white}>{value===ui.preference?'✓  ':''}{title}</Label>
      </Tile>)}
    {!!ui.error&&<Label size={24*s} color="#ffc591">{ui.error}</Label>}
  </View>;
  if(section&&!servicePage)return <Management key={section} section={section} t={t} language={ui.language}
    s={s} active={active} configured={configured} connectionNote={note} onBack={()=>setSection('')}
    onConnect={()=>{
      if(configured)return act(connect);
      setSection('');setNote('');setEdit(true);
    }} onServices={()=>setServicePage(true)}/>;
  if(servicePage)return <View style={body}>
    <View style={[row,{justifyContent:'space-between',marginBottom:12*s}]}>
      <Label size={48*s} bold>{t('服务分流')}</Label>
      <Tile s={s} action={back} style={{paddingHorizontal:25*s,paddingVertical:12*s}}><Label size={24*s}>{t('返回')}</Label></Tile>
    </View>
    <Label size={25*s} color={dim}>{t(mode==='global'?'全局代理中。以下选择已保存，切回智能分流后生效。':'按服务域名识别。跟随智能规则时，以下五项默认直连。')}</Label>
    <View style={[row,{marginTop:29*s,marginBottom:10*s,paddingHorizontal:19*s}]}>
      <View style={{width:240*s}}><Label size={21*s} color={dim}>{t('视频服务')}</Label></View>
      <View style={{flex:1}}><Label size={21*s} color={dim}>{t('智能模式设置')}</Label></View>
      <View style={{width:165*s}}><Label size={21*s} color={dim}>{t('最近出站')}</Label></View>
    </View>
    {services.map((service,index)=><View key={service} style={[row,{height:97*s,paddingHorizontal:19*s,borderTopWidth:1*s,borderColor:line}]}>
      <View style={{width:240*s}}><Label size={31*s} bold>{t(service)}</Label></View>
      <View style={[row,{flex:1}]}>{choicesList.map(([value,title])=>{
        const selected=choices[index]===value;
        return <Tile key={value} s={s} preferred={index===0&&value==='f'} action={()=>act(()=>{
          const current=Core.serviceChoices();
          const err=Core.setServiceChoices(current.slice(0,index)+value+current.slice(index+1));
          if(err)throw Error(err);
        })} style={{width:208*s,marginRight:14*s,paddingVertical:12*s,alignItems:'center',backgroundColor:selected?'#253e49':panel}}>
          <View style={row}><View style={{width:13*s,height:13*s,borderRadius:7*s,marginRight:9*s,borderWidth:2*s,borderColor:selected?green:dim,backgroundColor:selected?green:panel}}/>
          <Label size={24*s} bold={selected} color={selected?green:white}>{t(title)}</Label></View>
        </Tile>;
      })}</View>
      <View style={{width:165*s}}><Label size={26*s} color={evidence&&actual.services?.[index]==='direct'?green:dim}>{!active?'—':t(evidence?routeName(actual.services?.[index]):'待观察')}</Label></View>
    </View>)}
    <Label size={22*s} color={note?'#ffc591':dim} style={{marginTop:17*s}}>{diagnostic(t,note)||statusCaption+' · '+t('更改对新连接生效')}</Label>
  </View>;
  if(edit==='privacy')return <View style={body}>
    <View style={[row,{justifyContent:'space-between',marginBottom:32*s}]}>
      <Label size={48*s} bold>{t('隐私政策')}</Label>
      <Tile s={s} preferred action={back} style={{paddingHorizontal:25*s,paddingVertical:12*s}}><Label size={24*s}>{t('返回')}</Label></Tile>
    </View>
    {[
      ['电视上保存什么','订阅链接及访问凭据、节点缓存、分流规则、节点选择和最近出站状态保存在应用数据中。应用根据连接的目标域名和 IP 匹配规则。'],
      ['谁会收到网络请求','订阅和规则源接收更新请求。代理连接通过你选定的节点转发，直连连接到达目标服务。系统 VPN 接管 IPv4，IPv6 可能直接联网。'],
      ['DNS 与测速','应用向系统配置 Cloudflare DNS（1.1.1.1）。自动选点与延迟测试会访问 Google gstatic，下载测速会访问 Cloudflare Speed。相关服务可看到请求来源 IP。'],
      ['管理数据与联系','可随时断开 VPN。清除本应用的存储数据可移除本地配置和缓存。发布者未经营节点服务，应用不向发布者上传浏览记录。隐私咨询可使用商店页面列出的开发者联系方式。'],
    ].map(([title,detail])=><View key={title} style={{marginBottom:16*s}}>
      <Label bold size={29*s}>{t(title)}</Label><Label size={24*s} color={dim} style={{marginTop:5*s}}>{t(detail)}</Label>
    </View>)}
  </View>;
  if(edit)return <View style={[body,{justifyContent:'center',paddingHorizontal:175*s}]}>
    <Label size={53*s} bold>{t('订阅管理')}</Label><Label size={26*s} color={dim} style={{marginVertical:28*s}}>{t('粘贴 HTTPS 订阅链接，保存后连接。')}</Label>
    <TextInput secureTextEntry selectTextOnFocus autoCapitalize="none" autoCorrect={false} value={url} onChangeText={setUrl}
      placeholder={t('HTTPS 订阅链接')} placeholderTextColor={dim} style={{fontSize:30*s,color:white,padding:22*s,borderRadius:12*s,backgroundColor:panel,borderWidth:2*s,borderColor:line}}/>
    <View style={[row,{marginVertical:27*s}]}>{[[working?'正在连接…':'保存并连接',()=>act(save)],['隐私政策',()=>setEdit('privacy')],['返回',back]].map(([label,fn],i)=><Tile key={i} preferred={!i} s={s} action={fn} style={{paddingHorizontal:24*s,paddingVertical:15*s,marginRight:16*s}}><Label bold size={26*s}>{t(label)}</Label></Tile>)}</View>
    {!!note&&<Label size={25*s} color="#ffc591">{diagnostic(t,note)}</Label>}
  </View>;
  return <View style={body}>
    <View style={[row,{justifyContent:'space-between',marginBottom:35*s}]}>
      <View style={row}><View style={{width:8*s,height:46*s,borderRadius:4*s,backgroundColor:accent,marginRight:16*s}}/>
        <Label size={46*s} bold>Mihomo</Label><Label size={25*s} color={dim} style={{marginLeft:22*s}}>{t('电视 VPN')}</Label></View>
      <View style={row}><Tile s={s} action={()=>setSection('nodes')} style={{paddingHorizontal:23*s,paddingVertical:11*s,marginRight:16*s}}><Label size={24*s}>{t('节点')}</Label></Tile>
      <Tile s={s} action={()=>setSection('rules')} style={{paddingHorizontal:23*s,paddingVertical:11*s,marginRight:16*s}}><Label size={24*s}>{t('规则')}</Label></Tile>
      <Tile s={s} action={()=>{setEdit(true);setNote('');}} style={{paddingHorizontal:23*s,paddingVertical:11*s}}><Label size={24*s}>{t('订阅管理')}</Label></Tile>
      <Tile s={s} action={()=>setSection('language')} style={{marginLeft:16*s,paddingHorizontal:23*s,paddingVertical:11*s}}><Label size={24*s}>English / 中文</Label></Tile></View>
    </View>
    <View style={{flexDirection:'row'}}>
      <Tile s={s} preferred style={{flex:1,padding:32*s,marginRight:24*s,justifyContent:'space-between'}} action={()=>act(async()=>{
        if(!configured){setEdit(true);return;}
        if(Core.coreState()==='running')await disconnect();else await connect();
      })}>
        <View style={[row,{justifyContent:'space-between'}]}><Label size={26*s} color={dim}>{t('系统 VPN · IPv4')}</Label><Power s={s} active={active}/></View>
        <View><Label size={(ui.language==='en'?49:65)*s} bold>{networkStatus}</Label>
        <Label size={26*s} color={active?green:accent} style={{marginTop:14*s}}>{t(working?'正在处理…':active?'按确定键断开':configured?'按确定键连接':'按确定键导入订阅')}</Label></View>
      </Tile>
      <View style={{flex:1.37}}><Label size={26*s} color={dim} style={{marginBottom:13*s}}>{t('路由模式')}</Label>
        {modeTile('smart','国内域名和 IP 直连，其他走代理。')}
        {modeTile('global','局域网直连，其他使用 TV 节点组。')}
      </View>
    </View>
    <View style={{marginTop:32*s,borderTopWidth:2*s,borderColor:line,paddingTop:22*s}}>
      <View style={[row,{justifyContent:'space-between'}]}><Label bold size={31*s}>{t('最近的 B 站连接')}</Label>
      <Label size={22*s} color={dim}>{statusCaption}</Label></View>
      <View style={[row,{marginTop:20*s}]}>{[['接口',api],['封面',cover],['视频 CDN',media]].map(([label,route],i)=>
        <View key={label} style={{flex:1,marginRight:i<2?14*s:0,paddingHorizontal:18*s,paddingVertical:17*s,borderRadius:12*s,backgroundColor:panel}}>
          <Label size={22*s} color={dim}>{t(label)}</Label>
          <Label bold size={29*s} color={evidence&&route==='direct'?green:white} style={{marginTop:8*s}}>{!active?'—':t(evidence?routeName(route):'待观察')}</Label></View>)}</View>
    </View>
    {Boolean(note||state&&!stateNames[state])&&<Label size={23*s} color="#ffc591" style={{marginTop:14*s}}>{diagnostic(t,note||state)}</Label>}
  </View>;
}
AppRegistry.registerComponent(name,()=>App);
