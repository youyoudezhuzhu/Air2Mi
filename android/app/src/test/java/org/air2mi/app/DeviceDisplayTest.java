package org.air2mi.app;
import org.junit.Test;
import static org.junit.Assert.*;
public class DeviceDisplayTest {
 @Test public void rawCodeIsFilteredFromEveryMessage(){String code="AMD1-1234-1234-1234-1234-1234-1234-1234-1234";String value=DeviceDisplay.filter("设备码"+code);assertFalse(value.contains("AMD1"));assertTrue(value.contains("AVD1-"));assertEquals(44,DeviceDisplay.code(code).length());}
 @Test public void malformedCodeFailsClosed(){assertEquals("[device-code-unavailable]",DeviceDisplay.code("AMD1-INVALID"));}
 @Test public void enteredCDKIsNotReturnedInErrors(){assertEquals("[redacted]",DeviceDisplay.filter("A2M-12345-12345-12345-12345-12345-12345-12345-12345"));}
}
