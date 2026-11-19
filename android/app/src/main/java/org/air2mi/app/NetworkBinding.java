package org.air2mi.app;

import android.content.Context;
import android.net.ConnectivityManager;
import android.net.Network;
import android.net.NetworkCapabilities;
import android.net.LinkAddress;
import android.net.LinkProperties;
import java.net.Inet4Address;
import java.net.InetAddress;

final class NetworkBinding {
    private final ConnectivityManager manager;
    private final Network previous;
    private Network selected;
    NetworkBinding(Context context) {
        manager=(ConnectivityManager)context.getSystemService(Context.CONNECTIVITY_SERVICE);
        previous=manager.getBoundNetworkForProcess();
    }
    synchronized String refresh() {
        Network choice=null;
        for(Network network:manager.getAllNetworks()) {
            NetworkCapabilities caps=manager.getNetworkCapabilities(network);
            if(caps!=null&&caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI)){choice=network;break;}
        }
        if(choice==null)choice=manager.getActiveNetwork();
        if(!java.util.Objects.equals(choice,selected)) {
            if(!manager.bindProcessToNetwork(choice))throw new IllegalStateException("无法绑定 Android 网络");
            selected=choice;
        }
        LinkProperties properties=choice==null?null:manager.getLinkProperties(choice);
        if(properties!=null)for(LinkAddress address:properties.getLinkAddresses()) {
            InetAddress ip=address.getAddress();
            if(ip instanceof Inet4Address&&!ip.isLoopbackAddress())return ip.getHostAddress();
        }
        return null;
    }
    synchronized void close() {
        if(!manager.bindProcessToNetwork(previous))throw new IllegalStateException("无法恢复 Android 网络绑定");
        selected=null;
    }
}
