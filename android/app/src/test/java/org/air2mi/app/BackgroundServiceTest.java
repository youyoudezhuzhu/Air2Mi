package org.air2mi.app;
import android.app.NotificationManager;
import android.content.*;
import android.os.PowerManager;
import android.net.wifi.WifiManager;
import org.junit.*;
import org.junit.runner.RunWith;
import org.robolectric.*;
import org.robolectric.annotation.Config;
import org.robolectric.android.controller.ServiceController;
import static org.junit.Assert.*;
import static org.mockito.Mockito.*;
@RunWith(RobolectricTestRunner.class) @Config(sdk=34)
public class BackgroundServiceTest {
 private static class DirectExecutor extends java.util.concurrent.AbstractExecutorService {
  private boolean stopped;
  public void shutdown(){stopped=true;}public java.util.List<Runnable> shutdownNow(){stopped=true;return java.util.List.of();}
  public boolean isShutdown(){return stopped;}public boolean isTerminated(){return stopped;}public boolean awaitTermination(long timeout,java.util.concurrent.TimeUnit unit){return stopped;}
  public void execute(Runnable task){if(stopped)throw new java.util.concurrent.RejectedExecutionException();task.run();}
 }
 private ServiceController<EngineService> controller;private EngineService service;private EngineRuntime runtime;
 @Before public void setup(){controller=Robolectric.buildService(EngineService.class);service=controller.get();service.worker.shutdown();service.worker=new DirectExecutor();runtime=mock(EngineRuntime.class);when(runtime.shutdownProcess()).thenReturn(true);service.runtimeOverride=runtime;service.getSharedPreferences("engine-lifecycle",0).edit().clear().commit();controller.create();}
 @After public void cleanup()throws Exception{service.getSharedPreferences("engine-lifecycle",0).edit().putBoolean("wanted",false).commit();controller.destroy();verify(runtime,timeout(3000).atLeastOnce()).shutdownProcess();}
 @Test public void removingActivityTaskDoesNotStopEngine()throws Exception{service.onTaskRemoved(new Intent());verify(runtime,never()).stopHosted();verify(runtime,never()).shutdownProcess();}
 @Test public void stickyRestartOnlyResumesUserRequestedRun()throws Exception{
  service.getSharedPreferences("engine-lifecycle",0).edit().putBoolean("wanted",true).commit();
  assertEquals(android.app.Service.START_STICKY,service.onStartCommand(null,0,1));verify(runtime,timeout(3000)).startHosted();
 }
 @Test public void stopCommandClearsRecoveryPreferenceAndReleasesLocks()throws Exception{
  service.getSharedPreferences("engine-lifecycle",0).edit().putBoolean("wanted",true).commit();service.onStartCommand(new Intent().setAction("start"),0,1);verify(runtime,timeout(3000)).startHosted();
  java.lang.reflect.Field cpu=EngineService.class.getDeclaredField("cpu"),wifi=EngineService.class.getDeclaredField("wifi");cpu.setAccessible(true);wifi.setAccessible(true);
  assertTrue(((PowerManager.WakeLock)cpu.get(service)).isHeld());assertTrue(((WifiManager.WifiLock)wifi.get(service)).isHeld());
  assertEquals(android.app.Service.START_NOT_STICKY,service.onStartCommand(new Intent().setAction("stop"),0,2));verify(runtime,timeout(3000)).stopHosted();
  for(int i=0;i<50&&((PowerManager.WakeLock)cpu.get(service)).isHeld();i++)Thread.sleep(10);
  assertFalse(EngineService.wanted(service));assertFalse(((PowerManager.WakeLock)cpu.get(service)).isHeld());assertFalse(((WifiManager.WifiLock)wifi.get(service)).isHeld());
 }
 @Test public void startupFailureDoesNotLeakRunLocks()throws Exception{
  doThrow(new IllegalStateException("failed")).when(runtime).startHosted();service.getSharedPreferences("engine-lifecycle",0).edit().putBoolean("wanted",true).commit();service.onStartCommand(new Intent().setAction("start"),0,1);
  verify(runtime,timeout(3000)).recordFailure("failed");for(int i=0;i<50&&EngineService.wanted(service);i++)Thread.sleep(10);assertFalse(EngineService.wanted(service));assertEquals("failed",EngineService.phase());
 }
}
