package org.air2mi.app;

import android.Manifest;
import android.app.*;
import android.content.*;
import android.content.pm.PackageManager;
import android.content.pm.ServiceInfo;
import android.net.wifi.WifiManager;
import android.os.*;
import org.json.JSONObject;
import java.util.concurrent.*;

/** Owns Engine lifetime independently of the WebView/Activity. */
public final class EngineService extends Service {
    static final String CHANNEL="air2mi-engine";
    private static volatile String phase="stopped";
    ExecutorService worker=Executors.newSingleThreadExecutor();
    private PowerManager.WakeLock cpu;
    private WifiManager.WifiLock wifi;
    private volatile boolean destroyed;
    EngineRuntime runtimeOverride;
    private EngineRuntime runtime(){return runtimeOverride!=null?runtimeOverride:EngineRuntime.get(this);}
    static boolean wanted(Context c){return c.getSharedPreferences("engine-lifecycle",MODE_PRIVATE).getBoolean("wanted",false);}
    static String phase(){return phase;}
    static void command(Context c,String action){
        boolean start=!action.equals("stop");
        c.getSharedPreferences("engine-lifecycle",MODE_PRIVATE).edit().putBoolean("wanted",start).commit();
        Intent intent=new Intent(c,EngineService.class).setAction(action);
        c.startForegroundService(intent);
    }
    @Override public void onCreate(){super.onCreate();
        getSystemService(NotificationManager.class).createNotificationChannel(new NotificationChannel(CHANNEL,"Air2Mi 后台音频",NotificationManager.IMPORTANCE_LOW));
        startForeground(41,notification("正在准备音频服务"),ServiceInfo.FOREGROUND_SERVICE_TYPE_MEDIA_PLAYBACK|ServiceInfo.FOREGROUND_SERVICE_TYPE_CONNECTED_DEVICE);
    }
    private Notification notification(String message){
        PendingIntent open=PendingIntent.getActivity(this,0,new Intent(this,MainActivity.class),PendingIntent.FLAG_UPDATE_CURRENT|PendingIntent.FLAG_IMMUTABLE);
        PendingIntent stop=PendingIntent.getService(this,1,new Intent(this,EngineService.class).setAction("stop"),PendingIntent.FLAG_UPDATE_CURRENT|PendingIntent.FLAG_IMMUTABLE);
        return new Notification.Builder(this,CHANNEL).setSmallIcon(android.R.drawable.stat_sys_headset).setContentTitle("Air2Mi Engine").setContentText(message).setContentIntent(open).setOngoing(true).setOnlyAlertOnce(true).setCategory(Notification.CATEGORY_SERVICE).addAction(new Notification.Action.Builder(null,"停止内核",stop).build()).build();
    }
    private void update(String message){if(!destroyed)getSystemService(NotificationManager.class).notify(41,notification(message));}
    private void acquire(){
        if(cpu==null){cpu=getSystemService(PowerManager.class).newWakeLock(PowerManager.PARTIAL_WAKE_LOCK,"Air2Mi:audio");cpu.setReferenceCounted(false);}
        if(!cpu.isHeld())cpu.acquire();
        if(wifi==null){wifi=((WifiManager)getApplicationContext().getSystemService(WIFI_SERVICE)).createWifiLock(WifiManager.WIFI_MODE_FULL_HIGH_PERF,"Air2Mi:network");wifi.setReferenceCounted(false);}
        if(!wifi.isHeld())wifi.acquire();
    }
    private void release(){if(wifi!=null&&wifi.isHeld())wifi.release();if(cpu!=null&&cpu.isHeld())cpu.release();}
    @Override public int onStartCommand(Intent intent,int flags,int startId){
        String action=intent==null?(wanted(this)?"start":"stop"):intent.getAction();
        if(action==null)action="start";
        if(action.equals("stop"))getSharedPreferences("engine-lifecycle",MODE_PRIVATE).edit().putBoolean("wanted",false).commit();
        final String command=action;
        worker.execute(()->{
            try{
                if(command.equals("stop")||!wanted(this)){
                    phase="stopping";runtime().stopHosted();release();phase="stopped";
                    new Handler(getMainLooper()).post(()->{if(!wanted(this)){stopForeground(STOP_FOREGROUND_REMOVE);stopSelfResult(startId);}});
                }else{
                    phase="starting";acquire();
                    if(command.equals("restart"))runtime().stopHosted();
                    if(wanted(this)){runtime().startHosted();phase="running";update("音频内核在后台运行");}
                }
            }catch(Throwable error){
                runtime().recordFailure(error.getMessage());phase="failed";release();
                getSharedPreferences("engine-lifecycle",MODE_PRIVATE).edit().putBoolean("wanted",false).commit();
                update("内核启动或清理失败，请打开应用查看");
            }
        });
        return wanted(this)?START_STICKY:START_NOT_STICKY;
    }
    @Override public void onTaskRemoved(Intent rootIntent){super.onTaskRemoved(rootIntent);/* Activity dismissal is not an Engine stop. */}
    @Override public android.os.IBinder onBind(Intent intent){return null;}
    @Override public void onDestroy(){
        destroyed=true;
        worker.execute(()->{boolean cleaned=false;try{cleaned=runtime().shutdownProcess();}finally{release();phase=cleaned?"stopped":"failed";}});
        worker.shutdown();super.onDestroy();
    }
    static JSONObject backgroundState(Context context){
        JSONObject value=new JSONObject();try{
            PowerManager power=context.getSystemService(PowerManager.class);
            value.put("desired_running",wanted(context));value.put("service_phase",phase);
            value.put("battery_unrestricted",power.isIgnoringBatteryOptimizations(context.getPackageName()));
            value.put("notifications_allowed",Build.VERSION.SDK_INT<33||context.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS)==PackageManager.PERMISSION_GRANTED);
            value.put("foreground_service",phase.equals("starting")||phase.equals("running")||phase.equals("stopping"));
        }catch(Exception error){throw new IllegalStateException(error);}return value;
    }
}
