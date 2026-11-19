package org.air2mi.app;

import android.Manifest;
import android.app.Activity;
import android.content.*;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.os.*;
import android.webkit.*;
import org.json.JSONObject;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.*;

public final class MainActivity extends Activity {
 private static final int PICK_ENGINE=4102,EXPORT=4104,PERMISSIONS=4103,INSTALL_UPDATE=4106;
 private WebView web;
 private String exportJSON;
 private final ExecutorService requests=Executors.newFixedThreadPool(4);
 @Override public void onCreate(Bundle state){super.onCreate(state);
  android.content.SharedPreferences preferences=getSharedPreferences("engine-lifecycle",MODE_PRIVATE);
  if(preferences.getLong("update_expected_version",Long.MAX_VALUE)<=BuildConfig.VERSION_CODE){
   boolean resume=preferences.getBoolean("resume_after_update",false);preferences.edit().remove("update_expected_version").remove("resume_after_update").putBoolean("wanted",resume).commit();
  }
  if(state!=null)exportJSON=state.getString("export-json");
  requestPermissions();
  WebView.setWebContentsDebuggingEnabled(BuildConfig.DEBUG);
  web=new WebView(this);web.getSettings().setJavaScriptEnabled(true);web.getSettings().setDomStorageEnabled(true);
  web.getSettings().setAllowFileAccess(false);web.getSettings().setAllowContentAccess(false);web.getSettings().setSupportMultipleWindows(true);
  web.setWebViewClient(client(false));
  web.setWebChromeClient(new WebChromeClient(){@Override public boolean onCreateWindow(WebView view,boolean dialog,boolean gesture,Message message){
   if(!gesture)return false;
   WebView child=new WebView(MainActivity.this);child.setWebViewClient(client(true));((WebView.WebViewTransport)message.obj).setWebView(child);message.sendToTarget();return true;
  }});
  web.addJavascriptInterface(new Bridge(),"Air2MiNative");setContentView(EdgeToEdge.wrap(this,web));web.loadUrl("https://appassets.androidplatform.net/assets/index.html");
  if(EngineService.wanted(this)&&EngineService.phase().equals("stopped"))EngineService.command(this,"start");
 }
 private WebViewClient client(boolean popup){return new WebViewClient(){
  @Override public WebResourceResponse shouldInterceptRequest(WebView view,WebResourceRequest request){
   Uri uri=request.getUrl();
   if(!"https".equals(uri.getScheme())||!"appassets.androidplatform.net".equals(uri.getHost()))return null;
   String path=uri.getPath();
   if(!"GET".equals(request.getMethod())||path==null||!path.startsWith("/assets/")||path.startsWith("/assets/private/")||path.contains("..")||path.indexOf('\\')>=0)return new WebResourceResponse("text/plain","UTF-8",403,"Forbidden",java.util.Collections.emptyMap(),new java.io.ByteArrayInputStream(new byte[0]));
   try{
    String file=path.substring(8),extension=android.webkit.MimeTypeMap.getFileExtensionFromUrl(file);String mime=android.webkit.MimeTypeMap.getSingleton().getMimeTypeFromExtension(extension);
    if(file.endsWith(".js"))mime="application/javascript";if(mime==null)mime="application/octet-stream";
    java.util.Map<String,String> headers=new java.util.HashMap<>();headers.put("Cache-Control","no-store");headers.put("Content-Security-Policy","default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-src 'none'");
    return new WebResourceResponse(mime,"UTF-8",200,"OK",headers,getAssets().open(file));
   }catch(java.io.IOException error){return new WebResourceResponse("text/plain","UTF-8",404,"Not Found",java.util.Collections.emptyMap(),new java.io.ByteArrayInputStream(new byte[0]));}
  }

  @Override public boolean shouldOverrideUrlLoading(WebView view,WebResourceRequest request){return navigate(view,request.getUrl(),popup);}
  @Override public boolean shouldOverrideUrlLoading(WebView view,String url){return navigate(view,Uri.parse(url),popup);}
 };}
 private boolean navigate(WebView view,Uri uri,boolean popup){
  if(!popup&&"https".equals(uri.getScheme())&&"appassets.androidplatform.net".equals(uri.getHost())&&"/assets/index.html".equals(uri.getPath()))return false;
  if(("https".equals(uri.getScheme())||"http".equals(uri.getScheme()))&&uri.getHost()!=null&&uri.getUserInfo()==null){try{startActivity(new Intent(Intent.ACTION_VIEW,uri));}catch(android.content.ActivityNotFoundException error){android.widget.Toast.makeText(this,"没有可用的浏览器",0).show();}}
  if(popup)view.destroy();return true;
 }
 private void requestPermissions(){
  java.util.ArrayList<String> missing=new java.util.ArrayList<>();
  String[] permissions=Build.VERSION.SDK_INT>=33?new String[]{Manifest.permission.POST_NOTIFICATIONS,Manifest.permission.NEARBY_WIFI_DEVICES}:new String[]{Manifest.permission.ACCESS_FINE_LOCATION};
  for(String permission:permissions)if(checkSelfPermission(permission)!=PackageManager.PERMISSION_GRANTED)missing.add(permission);
  if(!missing.isEmpty())requestPermissions(missing.toArray(new String[0]),PERMISSIONS);
 }
 @Override public void onSaveInstanceState(Bundle state){super.onSaveInstanceState(state);state.putString("export-json",exportJSON);}
 @Override protected void onDestroy(){if(web!=null){web.removeJavascriptInterface("Air2MiNative");web.destroy();web=null;}requests.shutdown();super.onDestroy();}
 private void emit(String script){runOnUiThread(()->{if(web!=null)web.evaluateJavascript(script,null);});}
 @Override protected void onActivityResult(int requestCode,int resultCode,Intent data){super.onActivityResult(requestCode,resultCode,data);
  if(requestCode==INSTALL_UPDATE){resumeAfterInstaller();return;}
  if(resultCode!=RESULT_OK||data==null||data.getData()==null)return;
  Uri uri=data.getData();
  if(requestCode==EXPORT){String value=exportJSON;exportJSON=null;if(value!=null)requests.execute(()->{try(java.io.OutputStream out=getContentResolver().openOutputStream(uri,"wt")){if(out==null)throw new java.io.IOException("无法写入文件");out.write(value.getBytes(StandardCharsets.UTF_8));}catch(Exception error){emit("window.dispatchEvent(new CustomEvent('air2mi-import-error',{detail:"+JSONObject.quote(error.getMessage())+"}))");}});return;}
  if(requestCode!=PICK_ENGINE)return;
  try{getContentResolver().takePersistableUriPermission(uri,Intent.FLAG_GRANT_READ_URI_PERMISSION);}catch(SecurityException ignored){}
  requests.execute(()->{try{
   JSONObject result=EngineRuntime.get(this).importComponents(uri);
   emit("window.dispatchEvent(new CustomEvent('air2mi-engine-updated',{detail:"+result.toString()+"}))");
  }catch(Exception error){emit("window.dispatchEvent(new CustomEvent('air2mi-import-error',{detail:"+JSONObject.quote(DeviceDisplay.filter(error.getMessage()))+"}))");}});
 }
 private String request(String path,String method,String body){
  try{return new JSONObject().put("status",200).put("body",EngineRuntime.get(this).request(path,method,body)).toString();}
  catch(EngineRuntime.RequestFailure error){return DeviceDisplay.filter(error.envelope());}
  catch(Throwable error){try{return new JSONObject().put("status",500).put("body",new JSONObject().put("error",DeviceDisplay.filter(String.valueOf(error.getMessage())))).toString();}catch(Exception failure){return "{\"status\":500,\"body\":{\"error\":\"serialization failure\"}}";}}
 }
 private final class Bridge {
  @JavascriptInterface public void installUpdate(String component){requests.execute(()->{try{Intent intent=EngineRuntime.get(MainActivity.this).onlineUpdates(component).prepareInstall();runOnUiThread(()->{try{startActivityForResult(intent,INSTALL_UPDATE);}catch(Exception error){resumeAfterInstaller();android.widget.Toast.makeText(MainActivity.this,"无法打开系统安装器",0).show();}});}catch(Exception error){runOnUiThread(()->{resumeAfterInstaller();android.widget.Toast.makeText(MainActivity.this,DeviceDisplay.filter(error.getMessage()),1).show();});}});}
  @JavascriptInterface public void requestAsync(String id,String path,String method,String body){
   if(id.length()>80)return;
   try{requests.execute(()->{String result=request(path,method,body);emit("window.__air2miReply("+JSONObject.quote(id)+","+JSONObject.quote(result)+")");});}
   catch(RejectedExecutionException ignored){}
  }
  @JavascriptInterface public void pickEngine(){runOnUiThread(()->{Intent intent=new Intent(Intent.ACTION_OPEN_DOCUMENT).addCategory(Intent.CATEGORY_OPENABLE).setType("application/zip").addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION|Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION);startActivityForResult(intent,PICK_ENGINE);});}
  @JavascriptInterface public void copyText(String value){if(value.length()>262144)return;runOnUiThread(()->((android.content.ClipboardManager)getSystemService(CLIPBOARD_SERVICE)).setPrimaryClip(ClipData.newPlainText("Air2Mi",DeviceDisplay.filter(value))));}
  @JavascriptInterface public void exportDiagnostics(String value){if(value.length()>2097152)return;runOnUiThread(()->{exportJSON=DeviceDisplay.filter(value);startActivityForResult(new Intent(Intent.ACTION_CREATE_DOCUMENT).addCategory(Intent.CATEGORY_OPENABLE).setType("application/json").putExtra(Intent.EXTRA_TITLE,"air2mi-diagnostics.json"),EXPORT);});}
  @JavascriptInterface public void backgroundSettings(String mode){runOnUiThread(()->{Intent intent=mode.equals("notifications")?new Intent(android.provider.Settings.ACTION_APP_NOTIFICATION_SETTINGS).putExtra(android.provider.Settings.EXTRA_APP_PACKAGE,getPackageName()):new Intent(android.provider.Settings.ACTION_IGNORE_BATTERY_OPTIMIZATION_SETTINGS);try{startActivity(intent);}catch(android.content.ActivityNotFoundException error){startActivity(new Intent(android.provider.Settings.ACTION_APPLICATION_DETAILS_SETTINGS,Uri.parse("package:"+getPackageName())));}});}
 }
 private void resumeAfterInstaller(){android.content.SharedPreferences preferences=getSharedPreferences("engine-lifecycle",MODE_PRIVATE);boolean resume=preferences.getBoolean("resume_after_update",false);preferences.edit().remove("update_expected_version").remove("resume_after_update").commit();if(resume)EngineService.command(this,"start");}
}
