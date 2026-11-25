import React,{useCallback,useEffect,useState} from 'react';
import {AppState,BackHandler,Pressable,Text,TextInput,View} from 'react-native';
import Core from './src/NativeCore';

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
function delayText(r){return !r?.At?'待测':r.Error||r.Delay+' ms';}
function sourceLabel(e){
  if(e.Kind!=='feed')return e.Value;
  return e.Value.match(/^https:\/\/([^/?#@]+)(?:[/?#]|$)/i)?.[1]||'订阅地址';
}
export default function Management({section,s,onBack,onServices,active,onConnect}){
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
  const options=(map,sel,setter)=>buttons(Object.keys(map).map(key=>btn(map[key],()=>setter(key),{selected:sel===key})));
  function add(){
    const val=value.trim();if(!val){setError('请输入规则内容');return;}
    run('add',{Entry:{Kind:kind,Value:val,Target:target,Format:format,Behavior:behavior}},'list');
  }
  function openAdd(){setValue('');setTarget('DIRECT');setKind('suffix');setFormat('yaml');setBehavior('domain');setError('');setView('add');}
  const headers={list:nodes?'节点':'规则管理',detail:nodes?'节点详情':'规则详情',add:'添加规则',delete:'删除规则'};
  return <View style={{flex:1,backgroundColor:bg,paddingHorizontal:68*s,paddingTop:44*s}}>
    <View style={[row,{justifyContent:'space-between',marginBottom:12*s}]}>
      <Text style={{color:paper,fontSize:48*s,fontWeight:'700'}}>{headers[view]}</Text>{btn('返回',back)}
    </View>
    {!active ? <>{sub('连接 VPN 后可以读取订阅节点、测速和管理自定义规则。')}{buttons([btn('连接 VPN',onConnect,{preferred:true}),...(!nodes?[btn('视频服务',onServices)]:[])])}</> :
    view==='list'?<>
      {nodes?sub((data.selected?'手动选择':'自动选择')+'  ·  当前节点 '+(data.current||'正在读取'),true):sub('智能模式从上往下匹配。视频服务的指定线路优先。')}
      {buttons(nodes?[btn('自动选择',()=>run('select',{Name:''}),{selected:data.selected==='',preferred:!selected}),btn(data.job?'取消测速':'整组测延迟',()=>run(data.job?'cancel':'all'),{id:'group-test'})]:
        [btn('视频服务',onServices,{preferred:!selected}),btn('添加规则',openAdd)])}
      {Boolean(nodes&&data.notice)&&sub(data.notice)}
      <View style={{marginTop:8*s}}>{entries.slice(currentPage*pageSize,(currentPage+1)*pageSize).map((e,i)=>{
        const id=nodes?e.Name:e.ID;
        const detail=nodes?delayText(e.Latency)+(e.Latency?.At?'  ·  '+when(e.Latency.At):''):
          (e.Enabled?'已启用':'已停用')+'  ·  '+(e.Target==='TV'?'代理':'直连')+'  ·  '+kinds[e.Kind]+(e.Error?'  ·  '+e.Error:'');
        return <View key={id} style={row}>{btn((nodes?'':(currentPage*pageSize+i+1)+'. ')+(nodes?e.Name:sourceLabel(e)),()=>{
          setSelected(id);setError('');setView('detail');},{id,wide:true,detail,preferred:id===selected,selected:nodes&&id===data.current})}</View>;
      })}</View>
      {!entries.length&&sub(nodes?'正在读取节点…':'还没有自定义规则。当前使用内置的国内直连规则。')}
      {pageCount>1&&buttons([btn('上一页',()=>setPage(currentPage-1),{disabled:currentPage===0}),sub((currentPage+1)+' / '+pageCount),btn('下一页',()=>setPage(currentPage+1),{disabled:currentPage+1>=pageCount})])}
    </>:view==='add'?<>
      {options(kinds,kind,k=>{if(k!==kind)setValue('');setKind(k);})}
      {sub(kind==='feed'?'HTTPS 规则集链接':kind==='cidr'?'IPv4 网段，如 203.0.113.0/24':'域名，如 example.org')}
      <TextInput key={kind} autoCapitalize="none" autoCorrect={false} secureTextEntry={kind==='feed'} value={value} onChangeText={setValue}
        placeholder={kind==='feed'?'https://…':'输入匹配内容'} placeholderTextColor={muted}
        style={{color:paper,fontSize:29*s,borderColor:sky,borderWidth:2*s,borderRadius:12*s,padding:16*s,backgroundColor:panel}}/>
      {kind==='feed'&&<>{sub('文件格式')}{options({yaml:'YAML',text:'文本',mrs:'MRS'},format,f=>{setFormat(f);if(f==='mrs'&&behavior==='classical')setBehavior('domain');})}
        {sub('匹配类型')}{options(format==='mrs'?{domain:'域名',ipcidr:'IP 网段'}:behaviors,behavior,setBehavior)}</>}
      {sub('匹配后的线路')}{options({DIRECT:'直连',TV:'代理'},target,setTarget)}
      {buttons([btn('保存',add)])}
    </>:view==='delete'?<>{sub('删除 '+sourceLabel(chosen||{})+'？')}{buttons([btn('保留',back,{preferred:true}),btn('确认删除',()=>run('delete',{ID:selected},'list'))])}</>:
      !chosen?<>{sub('此项已从列表移除。')}{buttons([btn('返回列表',back,{preferred:true})])}</>:nodes?<>
       {sub(chosen.Name,true)}{sub('延迟  '+delayText(chosen.Latency)+'   '+when(chosen.Latency?.At))}
       {sub('下载  '+(chosen.Download?.At?(chosen.Download.Error||chosen.Download.Mbps.toFixed(1)+' Mbps'):'待测')+'   '+when(chosen.Download?.At))}
       {buttons([btn('使用此节点',()=>run('select',{Name:selected}),{preferred:true,selected:data.selected===selected}),
         ...Object.entries({latency:'测延迟',download:'测下载速度'}).map(([op,label])=>
           btn(data.job===op?'取消测速':label,()=>data.job===op?run('cancel'):run(op,{Name:selected}),
             {id:op,disabled:!!data.job&&data.job!==op})),
         ...(data.job==='all'?[btn('取消测速',()=>run('cancel'))]:[])])}
       {sub(data.job?'测速中…':'下载测试访问 Cloudflare，单次最多 4 MiB。')}
    </>:<>
      {sub(sourceLabel(chosen),true)}{sub(kinds[chosen.Kind]+'  ·  '+(chosen.Enabled?'已启用':'已停用')+'  ·  '+(chosen.Target==='TV'?'代理':'直连'))}
      {chosen.Kind==='feed'&&sub(chosen.Format.toUpperCase()+'  ·  '+behaviors[chosen.Behavior]+'  ·  '+chosen.Count+' 条  ·  '+(data.updating?.[chosen.ID]?'正在更新':when(chosen.Updated)||'等待更新'))}
      {Boolean(chosen.Error)&&sub(chosen.Error)}
      {buttons([btn(chosen.Enabled?'停用':'启用',()=>run('toggle',{ID:selected}),{id:'enabled',preferred:true}),btn(chosen.Target==='TV'?'改为直连':'改为代理',()=>run('target',{ID:selected}),{id:'target'}),
        btn('上移',()=>run('move',{ID:selected,Step:-1}),{disabled:entries[0]?.ID===selected}),btn('下移',()=>run('move',{ID:selected,Step:1}),{disabled:entries[entries.length-1]?.ID===selected}),
        ...(chosen.Kind==='feed'?[btn('立即更新',()=>run('update',{ID:selected}))]:[]),btn('删除',()=>setView('delete'))])}
    </>}
    {!!error&&sub(error)}
  </View>;
}
