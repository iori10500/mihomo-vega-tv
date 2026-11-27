import React,{useCallback,useEffect,useState} from 'react';
import {AppState,BackHandler,Pressable,Text,TextInput,View} from 'react-native';
import Core from './src/NativeCore';
import {diagnostic} from './i18n';

const paper='#f1f6fa', muted='#a8b8c7', mint='#82e5b8', sky='#79dbff', bg='#101722', panel='#1a2635';
const row={flexDirection:'row',alignItems:'center'};
const kinds={domain:'精确域名',suffix:'域名后缀',cidr:'IPv4 网段',feed:'规则集订阅'};
const behaviors={domain:'域名',ipcidr:'IP 网段',classical:'混合规则'};
const pageSize=5;
function call(Op,data={}){
  const response=JSON.parse(Core.manage(JSON.stringify({Op,...data})));
  if(response?.error)throw Error(response.error);
  return response;
}
function Button({title,onPress,s,preferred,wide,selected,disabled,detail}){
  const [focus,setFocus]=useState(false);
  return <Pressable focusable={!disabled} hasTVPreferredFocus={preferred} disabled={disabled}
    onFocus={()=>setFocus(true)} onBlur={()=>setFocus(false)} onPress={onPress}
    style={{flex:wide?1:undefined,marginRight:10*s,marginVertical:4*s,paddingHorizontal:18*s,height:(detail?77:56)*s,
      justifyContent:'center',borderRadius:12*s,borderWidth:3*s,borderColor:focus?sky:selected?mint:'#344557',
      backgroundColor:focus?'#213a4c':panel,opacity:disabled?.45:1}}>
    <Text numberOfLines={1} style={{fontSize:25*s,color:selected?mint:paper,fontWeight:focus?'700':'500'}}>{title}</Text>
    {Boolean(detail)&&<Text numberOfLines={1} style={{fontSize:19*s,color:muted,marginTop:3*s}}>{detail}</Text>}
  </Pressable>;
}
function when(ts){
  if(!ts)return '';
  const d=new Date(ts*1000),pad=n=>String(n).padStart(2,'0');
  return (d.getMonth()+1)+'/'+d.getDate()+' '+pad(d.getHours())+':'+pad(d.getMinutes());
}
function delayText(t,r){return !r?.At?t('待测'):diagnostic(t,r.Error)||r.Delay+' ms';}
function sourceLabel(t,e){
  if(e.Kind!=='feed')return e.Value;
  return e.Value.match(/^https:\/\/([^/?#@]+)(?:[/?#]|$)/i)?.[1]||t('订阅地址');
}
export default function Management({section,s,t,language,onBack,onServices,active,configured,connectionNote,onConnect}){
  const [view,setView]=useState('list'),[data,setData]=useState({}),[selected,setSelected]=useState(''),[page,setPage]=useState(0);
  const [visible,setVisible]=useState(AppState.currentState!=='background');
  useEffect(()=>{const sub=AppState.addEventListener('change',v=>setVisible(v==='active'));return ()=>sub.remove();},[]);
  const [kind,setKind]=useState('suffix'),[format,setFormat]=useState('yaml'),[behavior,setBehavior]=useState('domain');
  const [value,setValue]=useState(''),[target,setTarget]=useState('DIRECT'),[error,setError]=useState('');
  const nodes=section==='nodes', entries=(nodes?data.nodes:data.entries)||[];
  const chosen=entries.find(e=>(nodes?e.Name:e.ID)===selected);
  const pageCount=Math.max(1,Math.ceil(entries.length/pageSize)),currentPage=Math.min(page,pageCount-1);
  const refresh=useCallback(()=>{
    if(!active)return;
    try{setData(call(section));}catch(e){setError(e.message);}
  },[active,section]);
  useEffect(()=>{if(!visible)return;refresh();if(!active)return;const timer=setInterval(refresh,1500);return ()=>clearInterval(timer);},[refresh,active,visible]);
  function back(){setError('');if(view==='delete')setView('detail');else if(view!=='list')setView('list');else onBack();return true;}
  useEffect(()=>{const l=BackHandler.addEventListener('hardwareBackPress',back);return ()=>l.remove();},[view,onBack]);
  function run(op,args,next){try{call(op,args);setError('');refresh();if(next)setView(next);}catch(e){setError(e.message);}}
  const btn=(title,fn,options={})=><Button key={options.id||title} s={s} title={title} onPress={fn} {...options}/>;
  const sub=(str,accent=false)=><Text key={str} numberOfLines={2} style={{color:accent?mint:muted,fontSize:23*s,marginVertical:11*s}}>{str}</Text>;
  const buttons=(children)=><View style={[row,{flexWrap:'wrap'}]}>{children}</View>;
  const options=(map,sel,setter)=>buttons(Object.keys(map).map(key=>btn(t(map[key]),()=>setter(key),{selected:sel===key})));
  function add(){
    const val=value.trim();if(!val){setError('请输入规则内容');return;}
    run('add',{Entry:{Kind:kind,Value:val,Target:target,Format:format,Behavior:behavior}},'list');
  }
  function openAdd(){setValue('');setTarget('DIRECT');setKind('suffix');setFormat('yaml');setBehavior('domain');setError('');setView('add');}
  const headers={list:nodes?'节点':'规则管理',detail:nodes?'节点详情':'规则详情',add:'添加规则',delete:'删除规则'};
  return <View style={{flex:1,backgroundColor:bg,paddingHorizontal:68*s,paddingTop:44*s}}>
    <View style={[row,{justifyContent:'space-between',marginBottom:12*s}]}>
      <Text style={{color:paper,fontSize:48*s,fontWeight:'700'}}>{t(headers[view])}</Text>{btn(t('返回'),back)}
    </View>
    {!active ? <>{sub(t('连接 VPN 后可以读取订阅节点、测速和管理自定义规则。'))}{buttons([btn(t(configured?'连接 VPN':'添加订阅'),onConnect,{preferred:true}),...(!nodes?[btn(t('视频服务'),onServices)]:[])])}</> :
    view==='list'?<>
      {nodes?sub(t(data.selected?'手动选择':'自动选择')+'  ·  '+t('当前节点')+' '+(data.current||t('正在读取')),true):sub(t('智能模式从上往下匹配。视频服务的指定线路优先。'))}
      {buttons(nodes?[btn(t('自动选择'),()=>run('select',{Name:''}),{selected:data.selected==='',preferred:!selected}),btn(t(data.job?'取消测速':'整组测延迟'),()=>run(data.job?'cancel':'all'),{id:'group-test'})]:
        [btn(t('视频服务'),onServices,{preferred:!selected}),btn(t('添加规则'),openAdd)])}
      {Boolean(nodes&&data.notice)&&sub(diagnostic(t,data.notice))}
      <View style={{marginTop:8*s}}>{entries.slice(currentPage*pageSize,(currentPage+1)*pageSize).map((e,i)=>{
        const id=nodes?e.Name:e.ID;
        const detail=nodes?delayText(t,e.Latency)+(e.Latency?.At?'  ·  '+when(e.Latency.At):''):
          t(e.Enabled?'已启用':'已停用')+'  ·  '+t(e.Target==='TV'?'代理':'直连')+'  ·  '+t(kinds[e.Kind])+(e.Error?'  ·  '+diagnostic(t,e.Error):'');
        return <View key={id} style={row}>{btn((nodes?'':(currentPage*pageSize+i+1)+'. ')+(nodes?e.Name:sourceLabel(t,e)),()=>{
          setSelected(id);setError('');setView('detail');},{id,wide:true,detail,preferred:id===selected,selected:nodes&&id===data.current})}</View>;
      })}</View>
      {!entries.length&&sub(t(nodes?'正在读取节点…':'还没有自定义规则。当前使用内置的国内直连规则。'))}
      {pageCount>1&&buttons([btn(t('上一页'),()=>setPage(currentPage-1),{disabled:currentPage===0}),sub((currentPage+1)+' / '+pageCount),btn(t('下一页'),()=>setPage(currentPage+1),{disabled:currentPage+1>=pageCount})])}
    </>:view==='add'?<>
      {options(kinds,kind,k=>{if(k!==kind)setValue('');setKind(k);})}
      {sub(t(kind==='feed'?'HTTPS 规则集链接':kind==='cidr'?'IPv4 网段，如 203.0.113.0/24':'域名，如 example.org'))}
      <TextInput key={kind} autoCapitalize="none" autoCorrect={false} secureTextEntry={kind==='feed'} value={value} onChangeText={setValue}
        placeholder={kind==='feed'?'https://…':t('输入匹配内容')} placeholderTextColor={muted}
        style={{color:paper,fontSize:29*s,borderColor:sky,borderWidth:2*s,borderRadius:12*s,padding:16*s,backgroundColor:panel}}/>
      {kind==='feed'&&<>{sub(t('文件格式'))}{options({yaml:'YAML',text:'文本',mrs:'MRS'},format,f=>{setFormat(f);if(f==='mrs'&&behavior==='classical')setBehavior('domain');})}
        {sub(t('匹配类型'))}{options(format==='mrs'?{domain:'域名',ipcidr:'IP 网段'}:behaviors,behavior,setBehavior)}</>}
      {sub(t('匹配后的线路'))}{options({DIRECT:'直连',TV:'代理'},target,setTarget)}
      {buttons([btn(t('保存'),add)])}
    </>:view==='delete'?<>{sub(t('删除')+' '+sourceLabel(t,chosen||{})+(language==='en'?'?':'？'))}{buttons([btn(t('保留'),back,{preferred:true}),btn(t('确认删除'),()=>run('delete',{ID:selected},'list'))])}</>:
      !chosen?<>{sub(t('此项已从列表移除。'))}{buttons([btn(t('返回列表'),back,{preferred:true})])}</>:nodes?<>
       {sub(chosen.Name,true)}{sub(t('延迟')+'  '+delayText(t,chosen.Latency)+'   '+when(chosen.Latency?.At))}
       {sub(t('下载')+'  '+(chosen.Download?.At?(diagnostic(t,chosen.Download.Error)||chosen.Download.Mbps.toFixed(1)+' Mbps'):t('待测'))+'   '+when(chosen.Download?.At))}
       {buttons([btn(t('使用此节点'),()=>run('select',{Name:selected}),{preferred:true,selected:data.selected===selected}),
         ...Object.entries({latency:'测延迟',download:'测下载速度'}).map(([op,label])=>
           btn(t(data.job===op?'取消测速':label),()=>data.job===op?run('cancel'):run(op,{Name:selected}),
             {id:op,disabled:!!data.job&&data.job!==op})),
         ...(data.job==='all'?[btn(t('取消测速'),()=>run('cancel'))]:[])])}
       {sub(t(data.job?'测速中…':'下载测试访问 Cloudflare，单次最多 4 MiB。'))}
    </>:<>
      {sub(sourceLabel(t,chosen),true)}{sub(t(kinds[chosen.Kind])+'  ·  '+t(chosen.Enabled?'已启用':'已停用')+'  ·  '+t(chosen.Target==='TV'?'代理':'直连'))}
      {chosen.Kind==='feed'&&sub(chosen.Format.toUpperCase()+'  ·  '+t(behaviors[chosen.Behavior])+'  ·  '+chosen.Count+(language==='zh'?' 条':chosen.Count===1?' rule':' rules')+'  ·  '+(data.updating?.[chosen.ID]?t('正在更新'):when(chosen.Updated)||t('等待更新')))}
      {Boolean(chosen.Error)&&sub(diagnostic(t,chosen.Error))}
      {buttons([btn(t(chosen.Enabled?'停用':'启用'),()=>run('toggle',{ID:selected}),{id:'enabled',preferred:true}),btn(t(chosen.Target==='TV'?'改为直连':'改为代理'),()=>run('target',{ID:selected}),{id:'target'}),
        btn(t('上移'),()=>run('move',{ID:selected,Step:-1}),{disabled:entries[0]?.ID===selected}),btn(t('下移'),()=>run('move',{ID:selected,Step:1}),{disabled:entries[entries.length-1]?.ID===selected}),
        ...(chosen.Kind==='feed'?[btn(t('立即更新'),()=>run('update',{ID:selected}))]:[]),btn(t('删除'),()=>setView('delete'))])}
    </>}
    {!!(error||(!active&&connectionNote))&&sub(diagnostic(t,error||connectionNote))}
  </View>;
}
