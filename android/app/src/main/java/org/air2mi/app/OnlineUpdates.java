package org.air2mi.app;

import android.content.*;
import android.content.pm.*;
import android.net.Uri;
import android.os.Build;
import com.chaquo.python.*;
import com.chaquo.python.android.AndroidPlatform;
import java.io.*;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.security.MessageDigest;
import org.json.JSONObject;

/** Signed APK transport; native Engine updates on Android use system APK replacement. */
final class OnlineUpdates {
 private static final java.util.concurrent.atomic.AtomicBoolean downloadBusy=new java.util.concurrent.atomic.AtomicBoolean();
 private final Context context;private final PyObject client;private final String component;
 OnlineUpdates(Context c,String component)throws Exception{
  if(!component.equals("app")&&!component.equals("engine"))throw new IllegalArgumentException("更新组件无效");this.component=component;
  context=c.getApplicationContext();
  if(!Python.isStarted())Python.start(new AndroidPlatform(context));
  File code=new File(context.getCodeCacheDir(),"online-update-host");code.mkdirs();
  try(InputStream in=c.getAssets().open("air2mi_update_client.py");OutputStream out=new FileOutputStream(new File(code,"air2mi_update_client.py"))){byte[] b=new byte[8192];int n;while((n=in.read(b))!=-1)out.write(b,0,n);}
  String key;try(InputStream in=c.getAssets().open("engine-update-trust.pub")){key=new String(read(in),StandardCharsets.UTF_8).trim();}
  Python python=Python.getInstance();python.getModule("sys").get("path").callAttr("insert",0,code.getAbsolutePath());
  client=python.getModule("air2mi_update_client").callAttr("Client",new File(c.getFilesDir(),"online-updates-"+component).getAbsolutePath(),key,c.getPackageName(),BuildConfig.VERSION_CODE,null,false,component,BuildConfig.VERSION_NAME);
 }
 static byte[] read(InputStream in)throws IOException{ByteArrayOutputStream b=new ByteArrayOutputStream();byte[] part=new byte[8192];int n;while((n=in.read(part))!=-1)b.write(part,0,n);return b.toByteArray();}
 JSONObject check()throws Exception{return new JSONObject(client.callAttr("check",EngineRuntime.get(context).installedManifest()==null?"{}":EngineRuntime.get(context).installedManifest().toString()).toString());}
 JSONObject snapshot()throws Exception{return new JSONObject(client.callAttr("snapshot").toString());}
 void begin(String release)throws Exception{if(!downloadBusy.compareAndSet(false,true))throw new IOException("已有应用或内核更新正在下载");try{client.callAttr("begin",release);context.startForegroundService(new Intent(context,EngineUpdateService.class).putExtra("component",component));}catch(Exception error){downloadBusy.set(false);client.callAttr("failed",error.getMessage());throw error;}}
 void download()throws Exception{try{String file=client.callAttr("download").toString();if ("engine".equals(component) && new File(context.getFilesDir(), "online-updates-engine/engine-update.zip").equals(new File(file))) {
    File archive = new File(file);
    JSONObject result = EngineRuntime.get(context).importComponents(Uri.fromFile(archive));
    client.callAttr("imported",result.optBoolean("restart_required"));
    Files.deleteIfExists(archive.toPath());
   } else verifyCandidate(new File(file));}catch(Exception error){client.callAttr("failed",error.getMessage());throw error;}finally{downloadBusy.set(false);}}
 private JSONObject candidate()throws Exception{return new JSONObject(client.callAttr("candidate").toString());}
 File verifyCandidate(File file)throws Exception{
  File expected=new File(context.getFilesDir(),"online-updates-"+component+"/"+component+"-update.apk");
  if(!file.getCanonicalFile().equals(expected.getCanonicalFile())||!file.isFile())throw new IOException("更新文件位置无效");
  JSONObject metadata=candidate();MessageDigest hash=MessageDigest.getInstance("SHA-256");
  try(InputStream in=new FileInputStream(file)){byte[] block=new byte[65536];int n;while((n=in.read(block))!=-1)hash.update(block,0,n);}
  if(file.length()!=metadata.getLong("size")||!hex(hash.digest()).equals(metadata.getString("sha256")))throw new IOException("APK 校验失败");
  PackageManager pm=context.getPackageManager();PackageInfo archive=pm.getPackageArchiveInfo(file.getAbsolutePath(),PackageManager.GET_SIGNING_CERTIFICATES);
  PackageInfo current=pm.getPackageInfo(context.getPackageName(),PackageManager.GET_SIGNING_CERTIFICATES);
  if(archive==null||!context.getPackageName().equals(archive.packageName)||archive.getLongVersionCode()<=current.getLongVersionCode()||archive.getLongVersionCode()!=metadata.getLong("version_code"))throw new IOException("APK 包名或覆盖升级版本不匹配");
  if(archive.signingInfo==null||archive.signingInfo.getApkContentsSigners().length!=1||current.signingInfo==null||current.signingInfo.getApkContentsSigners().length!=1)throw new IOException("APK 签名信息无效");
  String signer=hex(MessageDigest.getInstance("SHA-256").digest(archive.signingInfo.getApkContentsSigners()[0].toByteArray()));
  String previous=hex(MessageDigest.getInstance("SHA-256").digest(current.signingInfo.getApkContentsSigners()[0].toByteArray()));
  if(!signer.equals(metadata.getString("signer_sha256"))||!signer.equals(previous))throw new IOException("APK 签名与当前应用不一致");
  return file;
 }
 Intent prepareInstall()throws Exception{
  if(!"install_ready".equals(snapshot().optString("phase")))throw new IOException("请先完成更新下载");
  if(Build.VERSION.SDK_INT>=26&&!context.getPackageManager().canRequestPackageInstalls())return new Intent(android.provider.Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES,Uri.parse("package:"+context.getPackageName()));
  verifyCandidate(new File(context.getFilesDir(),"online-updates-"+component+"/"+component+"-update.apk"));
  boolean resume=EngineService.wanted(context);
  context.getSharedPreferences("engine-lifecycle",0).edit().putBoolean("resume_after_update",resume).putLong("update_expected_version",candidate().getLong("version_code")).commit();
  EngineService.command(context,"stop");
  long until=System.currentTimeMillis()+20000;
  while((EngineRuntime.get(context).isRunning()||EngineService.phase().equals("stopping")||EngineService.phase().equals("starting")||EngineService.phase().equals("running"))&&System.currentTimeMillis()<until)Thread.sleep(100);
  if(EngineRuntime.get(context).isRunning()||EngineService.phase().equals("stopping")||EngineService.phase().equals("starting")||EngineService.phase().equals("running"))throw new IOException("停止旧内核超时，更新未开始");
  return new Intent(Intent.ACTION_INSTALL_PACKAGE,Uri.parse("content://"+context.getPackageName()+".engine-update/"+component+"-update.apk"))
   .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION).putExtra(Intent.EXTRA_RETURN_RESULT,true);
 }
 static String hex(byte[] bytes){StringBuilder s=new StringBuilder();for(byte b:bytes)s.append(String.format(java.util.Locale.ROOT,"%02x",b));return s.toString();}
}
