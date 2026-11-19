package org.air2mi.app;

import android.app.*;
import android.content.*;
import android.net.*;
import android.net.nsd.*;
import android.net.wifi.WifiManager;
import android.os.Build;
import android.provider.OpenableColumns;
import android.util.Log;
import com.chaquo.python.*;
import com.chaquo.python.android.AndroidPlatform;
import org.json.*;
import java.io.*;
import java.net.*;
import java.nio.file.*;
import java.security.MessageDigest;
import java.util.*;
import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.zip.*;

/** Android host/installer. Only the explicit local bundledTest variant embeds an Engine ZIP. */
final class EngineRuntime {
    private static EngineRuntime instance;
    static synchronized EngineRuntime get(Context context){if(instance==null)instance=new EngineRuntime(context.getApplicationContext());return instance;}
    private final Context context;
    private final File dataDir,installDir;
    private PyObject host;
    private JSONObject buildManifest;
    private JSONObject endpoint;
    private NetworkBinding network;
    private WifiManager.MulticastLock multicast;
    private ConnectivityManager.NetworkCallback callback;
    private final Map<Integer,Publication> publications=new ConcurrentHashMap<>();
    private final AtomicInteger ids=new AtomicInteger();
    private volatile String networkError="";
    private volatile boolean closing=true;
    private boolean componentsLoaded;
    private PyObject manager;
    private OnlineUpdates updater;
    private OnlineUpdates appUpdater;
 synchronized OnlineUpdates onlineUpdates()throws Exception{return onlineUpdates("engine");}
 synchronized OnlineUpdates onlineUpdates(String component)throws Exception{if("app".equals(component)){if(appUpdater==null)appUpdater=new OnlineUpdates(context,"app");return appUpdater;}if(!"engine".equals(component))throw new IllegalArgumentException("更新组件无效");if(updater==null)updater=new OnlineUpdates(context,"engine");return updater;}
    private volatile String lastError="";
    static final String APP_VERSION=BuildConfig.VERSION_NAME;
    private EngineRuntime(Context c){context=c;dataDir=new File(c.getFilesDir(),"engine-data");installDir=new File(c.getFilesDir(),"engine-components");applyPending();}

    JSONObject installedManifest(){return readJson(new File(installDir,"engine-build.json"));}
    synchronized void ensureBundledEngine()throws Exception{
        // Seed a fresh install. The installed component has its own update lifecycle.
        if (!BuildConfig.BUNDLED_TEST_ENGINE || installedManifest() != null) return;
        File archive=new File(context.getCacheDir(),"bundled-test-engine.zip");
        try{
            copyAsset("private/air2mi-engine-components.zip",archive);
            if(!sha256(archive).equals(BuildConfig.BUNDLED_TEST_ENGINE_SHA256))throw new IOException("内置测试内核校验失败");
            importComponents(Uri.fromFile(archive));
        }finally{Files.deleteIfExists(archive.toPath());}
    }
    synchronized boolean isRunning(){try{return host!=null&&!closing&&pyObject(host.callAttr("status")).optBoolean("running");}catch(Exception e){return false;}}

    synchronized JSONObject importComponents(Uri uri)throws Exception{

        File staging=new File(context.getFilesDir(),"engine-components-import");deleteTree(staging);File nativeDir=new File(staging,"native");nativeDir.mkdirs();
        byte[][] manifestBytes={null};Map<String,File> files=new HashMap<>();long[] total={0};
        try(InputStream raw=context.getContentResolver().openInputStream(uri)){
            if(raw==null)throw new IOException("无法读取所选组件包");
            ZipInputStream zip=new ZipInputStream(new BufferedInputStream(raw));ZipEntry entry;
            while((entry=zip.getNextEntry())!=null){
                String name=entry.getName();if(entry.isDirectory())continue;
                if(name.equals("air2mi-engine-host.aar")){
                    byte[] nested=readEntry(zip,total);
                    try(ZipInputStream aar=new ZipInputStream(new ByteArrayInputStream(nested))){ZipEntry item;while((item=aar.getNextEntry())!=null)if(!item.isDirectory())consumeComponent(item.getName(),aar,staging,nativeDir,files,manifestBytes,total);}
                } else consumeComponent(name,zip,staging,nativeDir,files,manifestBytes,total);
            }
        }
        if(manifestBytes[0]==null)throw new IOException("缺少 Engine 构建清单");
        JSONObject manifest=new JSONObject(new String(manifestBytes[0],java.nio.charset.StandardCharsets.UTF_8));
        if(!"android".equals(manifest.optString("platform"))||!"arm64-v8a".equals(manifest.optString("architecture")))throw new IOException("内核平台或架构不匹配（需要 Android ARM64-v8a）");
        if(manifest.optInt("minimum_android_api",29)>Build.VERSION.SDK_INT)throw new IOException("Android 系统版本低于内核要求");
        JSONObject expected=manifest.getJSONObject("files");
        for(String name:new String[]{manifest.getJSONObject("files").has("engine-native.zip")?"engine-native.zip":"engine-source.zip","libopenairplay2.so","libair2mi_codec.so"}){
            File f= name.endsWith(".zip")?new File(staging,name):files.get(name);
            if(f==null||!f.isFile())throw new IOException("组件包缺少 "+name);
            String hash=sha256(f);String key=name.startsWith("lib")?name:name;
            if(!hash.equalsIgnoreCase(expected.optString(key)))throw new IOException(name+" SHA256 不匹配");
        }
        if(!"v1".equals(manifest.optString("api_version")))throw new IOException("Engine API 版本不兼容");
        Files.write(new File(staging,"engine-build.json").toPath(),manifestBytes[0]);
        verifyComponents(staging);
        if(componentsLoaded){File pending=new File(context.getFilesDir(),"engine-components-pending");deleteTree(pending);Files.move(staging.toPath(),pending.toPath(),StandardCopyOption.ATOMIC_MOVE);return new JSONObject(manifest.toString()).put("restart_required",true);}

        File old=new File(context.getFilesDir(),"engine-components-old");deleteTree(old);
        if(installDir.exists())Files.move(installDir.toPath(),old.toPath(),StandardCopyOption.ATOMIC_MOVE);
        try{Files.move(staging.toPath(),installDir.toPath(),StandardCopyOption.ATOMIC_MOVE);}catch(Exception e){if(old.exists())Files.move(old.toPath(),installDir.toPath(),StandardCopyOption.ATOMIC_MOVE);throw e;}
        deleteTree(old);buildManifest=manifest;return manifest;
    }

    synchronized JSONObject initialize()throws Exception{try{return initializeImpl();}catch(Exception error){try{shutdownCore();}catch(Exception cleanup){error.addSuppressed(cleanup);}throw error;}}
    private JSONObject initializeImpl()throws Exception{
        if(host!=null)return pyObject(host.callAttr("version"));
        ensureBundledEngine();
        File sourceZip=new File(installDir,new File(installDir,"engine-native.zip").exists()?"engine-native.zip":"engine-source.zip"),codec=new File(installDir,"native/libair2mi_codec.so"),receiver=new File(installDir,"native/libopenairplay2.so");
        if(!sourceZip.isFile()||!codec.isFile()||!receiver.isFile())throw new IllegalStateException("请先导入 Android ARM64 Engine 组件包");
        verifyComponents(installDir);
        if(!Python.isStarted())Python.start(new AndroidPlatform(context));
        if(!componentsLoaded){componentsLoaded=true;System.load(codec.getAbsolutePath());System.load(receiver.getAbsolutePath());}
        cleanupNetwork();closing=false;if(networkEvents==null||networkEvents.isShutdown())networkEvents=Executors.newSingleThreadExecutor();networkError="";network=new NetworkBinding(context);network.refresh();
        WifiManager wifi=(WifiManager)context.getSystemService(Context.WIFI_SERVICE);multicast=wifi.createMulticastLock("Air2Mi");multicast.setReferenceCounted(false);multicast.acquire();
        File bootstrapDir=new File(context.getCodeCacheDir(),"air2mi-host");bootstrapDir.mkdirs();copyAsset("air2mi_android_bootstrap.py",new File(bootstrapDir,"air2mi_android_bootstrap.py"));
        Python py=Python.getInstance();py.getModule("sys").get("path").callAttr("insert",0,bootstrapDir.getAbsolutePath());
        host=py.getModule("air2mi_android_bootstrap").callAttr("initialize","{}",dataDir.getAbsolutePath(),installDir.getAbsolutePath(),new File(installDir,"native").getAbsolutePath(),this);
        final PyObject owned=host;ConnectivityManager cm=(ConnectivityManager)context.getSystemService(Context.CONNECTIVITY_SERVICE);
        callback=new ConnectivityManager.NetworkCallback(){private void refresh(){if(closing)return;try{ExecutorService executor=networkEvents;if(executor==null)return;executor.execute(()->{synchronized(EngineRuntime.this){if(!closing&&host==owned)try{host.callAttr("network_changed",network.refresh());}catch(Throwable t){networkError=t.toString();Log.e("Air2Mi","network change",t);}}});}catch(RejectedExecutionException ignored){}}
            @Override public void onAvailable(Network n){refresh();}@Override public void onLost(Network n){refresh();}@Override public void onLinkPropertiesChanged(Network n,LinkProperties p){refresh();}};
        cm.registerDefaultNetworkCallback(callback);return pyObject(host.callAttr("version"));
    }
    private volatile ExecutorService networkEvents;
    synchronized JSONObject startHosted()throws Exception{try{if(manager!=null)manager.get("tone").callAttr("ensure_clean");initialize();host.callAttr("start");endpoint=pyObject(host.callAttr("get_api_endpoint"));lastError="";return status();}catch(Exception error){lastError=error.getMessage();shutdownCore();throw error;}}
    void stopHosted()throws Exception{stopTone();synchronized(this){shutdownCore();}}
    synchronized void recordFailure(String message){lastError=message;log("ERROR",message);}
    private synchronized void log(String level,String message){try{
        dataDir.mkdirs();File file=new File(dataDir,"app.log");if(file.length()>1048576){File old=new File(dataDir,"app.previous.log");Files.move(file.toPath(),old.toPath(),StandardCopyOption.REPLACE_EXISTING);}
        try(FileOutputStream out=new FileOutputStream(file,true)){out.write((java.time.Instant.now()+" "+level+" "+DeviceDisplay.filter(String.valueOf(message))+"\n").getBytes(java.nio.charset.StandardCharsets.UTF_8));}
    }catch(IOException error){Log.e("Air2Mi","Application log write failed",error);}}
    private void stopTone(){if(manager!=null)manager.callAttr("stop_tone");}
    private void shutdownCore(){
        closing=true;endpoint=null;
        if(host!=null){host.callAttr("shutdown");host.close();host=null;}
        cleanupNetwork();
    }
    public synchronized void suspendForTone(){if(host==null)throw new IllegalStateException("请先启动 Engine");host.callAttr("shutdown");host.close();host=null;endpoint=null;}
    public synchronized void resumeAfterTone()throws Exception{cleanupNetwork();if(EngineService.wanted(context))startHosted();}

    synchronized JSONObject status(){JSONObject out=new JSONObject();try{out.put("initialized",host!=null);out.put("running",isRunning());out.put("network_error",networkError);out.put("manifest",installedManifest());out.put("last_error",lastError);if(host!=null)out.put("engine",pyObject(host.callAttr("status")));}catch(Exception e){try{out.put("error",e.toString());}catch(Exception ignored){}}return out;}
    boolean shutdownProcess(){try{stopHosted();return true;}catch(Throwable error){lastError=error.getMessage();Log.e("Air2Mi","Engine cleanup failed",error);return false;}}
    private synchronized PyObject management()throws Exception{
        if(manager==null){
            if(!Python.isStarted())Python.start(new AndroidPlatform(context));
            File source=new File(context.getCodeCacheDir(),"air2mi-app-host");source.mkdirs();
            copyAsset("air2mi_app_host.py",new File(source,"air2mi_app_host.py"));
            Python py=Python.getInstance();py.getModule("sys").get("path").callAttr("insert",0,source.getAbsolutePath());
            manager=py.getModule("air2mi_app_host").callAttr("Manager",this,dataDir.getAbsolutePath(),new File(installDir,"native").getAbsolutePath());
        }return manager;
    }
    public String callEngine(String path,String method,String body){
        try{return new JSONObject().put("status",200).put("body",engineRequest(path,method,body)).toString();}
        catch(RequestFailure error){return error.envelope();}
        catch(Exception error){return new RequestFailure(503,error.getMessage()).envelope();}
    }
    private JSONObject verifyComponents(File folder)throws Exception{
        JSONObject manifest=readJson(new File(folder,"engine-build.json"));
        if(manifest==null||!"android".equals(manifest.optString("platform"))||!"arm64-v8a".equals(manifest.optString("architecture"))||!"v1".equals(manifest.optString("api_version")))throw new IOException("内核平台、架构或 API 不匹配");
        if(manifest.optInt("minimum_android_api",29)>Build.VERSION.SDK_INT)throw new IOException("Android 版本低于内核要求");
        JSONObject hashes=manifest.getJSONObject("files");String archive=hashes.has("engine-native.zip")?"engine-native.zip":"engine-source.zip";
        for(String name:new String[]{archive,"libopenairplay2.so","libair2mi_codec.so"}){
            File file=new File(folder,name.startsWith("lib")?"native/"+name:name);
            if(!file.isFile()||!sha256(file).equalsIgnoreCase(hashes.optString(name)))throw new IOException("组件校验失败："+name);
            if(name.startsWith("lib")){byte[] header=new byte[20];try(InputStream input=new FileInputStream(file)){if(input.read(header)!=20)throw new IOException("ELF header missing");}
                if(header[0]!=127||header[1]!='E'||header[2]!='L'||header[3]!='F'||header[4]!=2||header[5]!=1||(header[18]&255)!=183||header[19]!=0)throw new IOException("需要 Android ARM64 原生组件");}
        }return manifest;
    }
    private void applyPending(){
        File pending=new File(context.getFilesDir(),"engine-components-pending"),old=new File(context.getFilesDir(),"engine-components-old");
        try{
            if(!installDir.exists()&&!pending.exists()&&old.exists())Files.move(old.toPath(),installDir.toPath(),StandardCopyOption.ATOMIC_MOVE);
            if(!pending.exists())return;
            verifyComponents(pending);
            deleteTree(old);if(installDir.exists())Files.move(installDir.toPath(),old.toPath(),StandardCopyOption.ATOMIC_MOVE);
            try{Files.move(pending.toPath(),installDir.toPath(),StandardCopyOption.ATOMIC_MOVE);}catch(Exception error){if(old.exists())Files.move(old.toPath(),installDir.toPath(),StandardCopyOption.ATOMIC_MOVE);throw error;}
        }catch(Exception error){lastError="内核更新未完成："+error.getMessage();}
    }


    public String getBindAddress(){try{return network==null?null:network.refresh();}catch(Exception e){networkError=e.toString();return null;}}
    public int beginPublish(String description)throws Exception{
        if(closing)throw new IllegalStateException("Host stopping");
        JSONObject data=new JSONObject(description);NsdServiceInfo info=new NsdServiceInfo();info.setServiceName(data.getString("name"));info.setServiceType("_airplay._tcp.");info.setPort(data.getInt("port"));
        JSONArray txt=data.optJSONArray("txt");if(txt!=null)for(int i=0;i<txt.length();i++){String item=txt.getString(i);int at=item.indexOf('=');info.setAttribute(at<0?item:item.substring(0,at),at<0?"":item.substring(at+1));}
        int id=ids.incrementAndGet();Publication p=new Publication();NsdManager manager=(NsdManager)context.getSystemService(Context.NSD_SERVICE);
        p.listener=new NsdManager.RegistrationListener(){public void onServiceRegistered(NsdServiceInfo x){p.state="registered";if(p.cancelled||closing)try{manager.unregisterService(p.listener);}catch(Exception e){p.state="unregister_failed:"+e.getClass().getSimpleName();}}
            public void onRegistrationFailed(NsdServiceInfo x,int code){p.state="registration_failed:"+code;publications.remove(id,p);p.removed.countDown();}
            public void onServiceUnregistered(NsdServiceInfo x){p.state="removed";p.removed.countDown();publications.remove(id);}
            public void onUnregistrationFailed(NsdServiceInfo x,int code){p.state="unregister_failed:"+code;}};
        synchronized(publications){if(closing)throw new IllegalStateException("Host stopping");publications.put(id,p);try{manager.registerService(info,NsdManager.PROTOCOL_DNS_SD,p.listener);}catch(RuntimeException error){publications.remove(id,p);p.removed.countDown();throw error;}}return id;
    }
    public String publicationState(int id){Publication p=publications.get(id);return p==null?"removed":p.state;}
    public void unpublish(int id){Publication p=publications.get(id);if(p==null)return;p.cancelled=true;try{if(!"pending".equals(p.state))((NsdManager)context.getSystemService(Context.NSD_SERVICE)).unregisterService(p.listener);if(!p.removed.await(2,TimeUnit.SECONDS))throw new IllegalStateException("NSD unregister timeout");}catch(InterruptedException e){Thread.currentThread().interrupt();throw new IllegalStateException(e);}}
    private static final class Publication{volatile String state="pending";volatile boolean cancelled;final CountDownLatch removed=new CountDownLatch(1);NsdManager.RegistrationListener listener;}

    public String getDeviceIdentity(){
        String value=android.provider.Settings.Secure.getString(context.getContentResolver(),android.provider.Settings.Secure.ANDROID_ID);
        if(value==null||value.isEmpty())throw new IllegalStateException("Stable Android ID unavailable");
        return value;
    }

    JSONObject request(String path,String method,String body){try{
        ensureBundledEngine();
        if(path.length()>1024||body.length()>131072||!path.startsWith("/api/")||path.contains("..")||path.contains("\\"))throw new RequestFailure(400,"无效请求");
        if(!Arrays.asList("GET","POST","PATCH","DELETE").contains(method))throw new RequestFailure(405,"不支持的请求方法");
        JSONObject value;
        if("/api/session".equals(path)&&"GET".equals(method))value=new JSONObject().put("csrf","android-local").put("app_version",APP_VERSION);
        else if("/api/state".equals(path)&&"GET".equals(method))value=snapshot();
        else if("/api/background".equals(path)&&"GET".equals(method))value=EngineService.backgroundState(context);
        else if("/api/app-updates".equals(path)&&"GET".equals(method))value=onlineUpdates("app").check();
        else if("/api/app-update-status".equals(path)&&"GET".equals(method))value=onlineUpdates("app").snapshot();
        else if("/api/app-update-online".equals(path)&&"POST".equals(method)){onlineUpdates("app").begin(new JSONObject(body).getString("release_id"));value=onlineUpdates("app").snapshot();}
        else if("/api/updates".equals(path)&&"GET".equals(method))value=onlineUpdates().check();
        else if("/api/update-status".equals(path)&&"GET".equals(method))value=onlineUpdates().snapshot();
        else if("/api/update-online".equals(path)&&"POST".equals(method)){onlineUpdates().begin(new JSONObject(body).getString("release_id"));value=onlineUpdates().snapshot();}
        else if(path.startsWith("/api/lifecycle/")&&"POST".equals(method)){
            String action=path.substring("/api/lifecycle/".length());
            if(!Arrays.asList("start","stop","restart").contains(action))throw new RequestFailure(404,"未知 Engine 操作");
            if(!action.equals("stop")&&manager!=null&&manager.get("tone").callAttr("busy").toBoolean())throw new RequestFailure(409,"请先停止测试音");
            EngineService.command(context,action);value=snapshot();
        }else if("/api/doctor".equals(path)&&"POST".equals(method)){
            synchronized(this){boolean temporary=host==null;try{initialize();JSONObject diagnostics=pyObject(host.callAttr("diagnostics"));value=new JSONObject().put("output",diagnostics.toString(2));}finally{if(temporary&&!EngineService.wanted(context))shutdownCore();}}
        }else if("/api/diagnostics".equals(path)&&"GET".equals(method))value=new JSONObject().put("snapshot",snapshot()).put("background",EngineService.backgroundState(context));
        else {
            JSONObject envelope=new JSONObject(management().callAttr("request",path,method,body).toString());
            if(envelope.getInt("status")>=400)throw new RequestFailure(envelope.getInt("status"),envelope.getJSONObject("body"));
            value=envelope.getJSONObject("body");
        }
        if(!method.equals("GET"))log("INFO",method+" "+path);
        return sanitize(value);
    }catch(RequestFailure error){throw error;}catch(Throwable error){throw new RequestFailure(422,error.getMessage()==null?error.toString():error.getMessage());}}
    private JSONObject sanitize(JSONObject value)throws Exception{management();return new JSONObject(Python.getInstance().getModule("air2mi_app_host").callAttr("public_json",value.toString()).toString());}

    private JSONObject snapshot()throws Exception{
        JSONObject manifest=installedManifest(),out=new JSONObject();
        JSONObject configuration=pyObject(management().callAttr("configuration"));
        boolean running=isRunning();
        out.put("app_version",APP_VERSION);out.put("engine_installed",manifest!=null);out.put("engine_manifest",manifest==null?new JSONObject():new JSONObject(manifest.toString()).put("source_commit",manifest.optString("source_revision")));
        out.put("engine_running",running);out.put("engine_error",lastError);out.put("operation",EngineService.phase());out.put("bind",getBindAddress());
        out.put("background",EngineService.backgroundState(context));out.put("test_tone",pyObject(management().get("tone").callAttr("snapshot")));
        out.put("environment",new JSONObject().put("platform","android").put("architecture","arm64-v8a").put("android_api",Build.VERSION.SDK_INT));
        out.put("configuration_cached",!running);
        out.put("state",running?engineRequest("/v1/state","GET",""):new JSONObject());
        for(String kind:new String[]{"inputs","speakers","routes"})out.put(kind,running?engineRequest("/v1/"+kind,"GET",""):new JSONObject().put(kind,configuration.optJSONArray(kind)==null?new JSONArray():configuration.getJSONArray(kind)));
        out.put("capabilities",new JSONObject().put("registry",true).put("single_target_routes",true).put("multi_target_routes",false).put("online_updates",true).put("test_tone",true).put("logs",true));
        out.put("updates",onlineUpdates().snapshot());out.put("app_updates",onlineUpdates("app").snapshot());return out;
    }
    private JSONObject engineRequest(String path,String method,String body)throws Exception{
        if(host==null||!isRunning())throw new RequestFailure(503,"Engine 尚未启动");endpoint=pyObject(host.callAttr("get_api_endpoint"));if(endpoint==null)throw new IllegalStateException("Engine 尚未启动");
        URL url=new URL(endpoint.getString("url")+path);HttpURLConnection c=(HttpURLConnection)url.openConnection();c.setConnectTimeout(4000);c.setReadTimeout(15000);c.setRequestMethod(method);c.setRequestProperty("Authorization","Bearer "+endpoint.getString("token"));
        if(body!=null&&!body.isEmpty()&&!"GET".equals(method)){c.setDoOutput(true);c.setRequestProperty("Content-Type","application/json");try(OutputStream o=c.getOutputStream()){o.write(body.getBytes(java.nio.charset.StandardCharsets.UTF_8));}}
        int status=c.getResponseCode();InputStream in=status<400?c.getInputStream():c.getErrorStream();byte[] data=in==null?new byte[0]:readLimit(in,2*1024*1024);c.disconnect();JSONObject value=data.length==0?new JSONObject():new JSONObject(new String(data,java.nio.charset.StandardCharsets.UTF_8));if(status>=400)throw new RequestFailure(status,value);return value;
    }

    private void consumeComponent(String name,InputStream in,File staging,File nativeDir,Map<String,File> files,byte[][] manifestBytes,long[] total)throws IOException{
        if(name.startsWith("/")||name.contains("..")||name.indexOf('\\')>=0)throw new IOException("组件包包含不安全路径");
        String leaf=null;
        if(name.equals("assets/engine-build.json"))leaf="engine-build.json";
        else if(name.equals("assets/engine-source.zip"))leaf="engine-source.zip";
        else if(name.equals("assets/engine-native.zip"))leaf="engine-native.zip";
        else if(name.equals("jniLibs/arm64-v8a/libopenairplay2.so")||name.equals("jni/arm64-v8a/libopenairplay2.so"))leaf="libopenairplay2.so";
        else if(name.equals("jniLibs/arm64-v8a/libair2mi_codec.so")||name.equals("jni/arm64-v8a/libair2mi_codec.so"))leaf="libair2mi_codec.so";
        if(leaf==null){drainEntry(in,total);return;}
        File target=leaf.startsWith("lib")?new File(nativeDir,leaf):new File(staging,leaf);
        try(FileOutputStream out=new FileOutputStream(target)){copyEntry(in,out,total);out.getFD().sync();}
        if(leaf.equals("engine-build.json"))manifestBytes[0]=Files.readAllBytes(target.toPath());else files.put(leaf,target);
    }
    private static byte[] readEntry(InputStream in,long[] total)throws IOException{ByteArrayOutputStream out=new ByteArrayOutputStream();copyEntry(in,out,total);return out.toByteArray();}
    private static void drainEntry(InputStream in,long[] total)throws IOException{byte[] b=new byte[65536];int n;while((n=in.read(b))!=-1){total[0]+=n;if(total[0]>400L*1024*1024)throw new IOException("组件包超过 400MB 限制");}}
    private static void copyEntry(InputStream in,OutputStream out,long[] total)throws IOException{byte[] b=new byte[65536];int n;while((n=in.read(b))!=-1){total[0]+=n;if(total[0]>400L*1024*1024)throw new IOException("组件包超过 400MB 限制");out.write(b,0,n);}}
    private void cleanupNetwork(){
        closing=true;RuntimeException failure=null;
        if(networkEvents!=null){networkEvents.shutdownNow();networkEvents=null;}
        if(callback!=null)try{((ConnectivityManager)context.getSystemService(Context.CONNECTIVITY_SERVICE)).unregisterNetworkCallback(callback);callback=null;}catch(RuntimeException error){failure=error;}
        for(Integer id:new ArrayList<>(publications.keySet()))try{unpublish(id);}catch(RuntimeException error){if(failure==null)failure=error;else failure.addSuppressed(error);}
        try{if(multicast!=null&&multicast.isHeld())multicast.release();multicast=null;}catch(RuntimeException error){if(failure==null)failure=error;else failure.addSuppressed(error);}
        try{if(network!=null)network.close();network=null;}catch(RuntimeException error){if(failure==null)failure=error;else failure.addSuppressed(error);}
        if(failure!=null)throw failure;
    }

    private void copyAsset(String name,File target)throws IOException{try(InputStream in=context.getAssets().open(name);OutputStream out=new FileOutputStream(target)){byte[] b=new byte[8192];int n;while((n=in.read(b))>0)out.write(b,0,n);}}
    private static JSONObject pyObject(PyObject p)throws JSONException{return new JSONObject(Python.getInstance().getModule("json").callAttr("dumps",p).toString());}
    private static JSONObject readJson(File f){try{return f.isFile()?new JSONObject(new String(Files.readAllBytes(f.toPath()),java.nio.charset.StandardCharsets.UTF_8)):null;}catch(Exception e){return null;}}
    private static byte[] readLimit(InputStream in,int limit)throws IOException{try(ByteArrayOutputStream out=new ByteArrayOutputStream()){byte[] b=new byte[16384];int n,total=0;while((n=in.read(b))!=-1){total+=n;if(total>limit)throw new IOException("Engine response too large");out.write(b,0,n);}return out.toByteArray();}}
    private static String sha256(File f)throws Exception{MessageDigest md=MessageDigest.getInstance("SHA-256");try(InputStream in=new FileInputStream(f)){byte[] b=new byte[65536];int n;while((n=in.read(b))!=-1)md.update(b,0,n);}StringBuilder s=new StringBuilder();for(byte b:md.digest())s.append(String.format(java.util.Locale.ROOT,"%02x",b));return s.toString();}
    private static void deleteTree(File f)throws IOException{if(!f.exists())return;File[] children=f.listFiles();if(children!=null)for(File c:children)deleteTree(c);if(!f.delete())throw new IOException("无法清理导入临时目录");}
    static final class RequestFailure extends RuntimeException{
        final int status;final JSONObject body;
        RequestFailure(int code,String message){this(code,errorBody(message));}
        RequestFailure(int code,JSONObject value){super(value.optString("detail",value.optString("error","操作失败")));status=code;body=value;}
        private static JSONObject errorBody(String message){JSONObject value=new JSONObject();try{value.put("error",message==null?"操作失败":message);}catch(Exception ignored){}return value;}
        String envelope(){try{return new JSONObject().put("status",status).put("body",body).toString();}catch(Exception error){return "{\"status\":500,\"body\":{\"error\":\"serialization failure\"}}";}}
    }
}
