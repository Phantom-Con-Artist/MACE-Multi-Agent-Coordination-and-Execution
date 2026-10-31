package io.github.phantomconartist.mace.data

import kotlinx.coroutines.runBlocking
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test

class HubTest {
    private val server = MockWebServer()

    @Before fun start() = server.start()

    @After fun stop() = server.close()

    @Test fun parsesTheHubsSignInLink() {
        val link = SignInLink.parse(" https://fedora.tail.ts.net/#connect-once=0123456789abcdef0123 ")
        assertEquals("https://fedora.tail.ts.net", link?.origin)
        assertEquals("0123456789abcdef0123", link?.code)
        assertEquals(SignInLink("https://fedora.tail.ts.net", ""), SignInLink.parse("fedora.tail.ts.net"))
        assertEquals(SignInLink("http://100.94.153.17:8080", ""), SignInLink.parse("http://100.94.153.17:8080/"))
        assertNull(SignInLink.parse("not a hub"))
        assertNull(SignInLink.parse("connect-once=0123456789abcdef"))
    }

    @Test fun signsInAndSendsTheDeviceCredential() = runBlocking {
        server.enqueue(MockResponse.Builder().body("""{"credential":"dev_1.abc","api":1,"name":"fedora","extra":true}""").build())
        server.enqueue(MockResponse.Builder().body("""[{"id":"r1","name":"Team","members":["a"],"roles":{}}]""").build())
        val origin = server.url("/").toString().trimEnd('/')
        val grant = Hub(origin).signIn("code", "MACE on Test")
        assertEquals("dev_1.abc", grant.credential)
        val body = server.takeRequest().body?.utf8().orEmpty()
        assertTrue(body.contains("\"code\":\"code\"") && body.contains("MACE on Test"))

        val rooms = Hub(origin, grant.credential).rooms()
        assertEquals("Team", rooms.single().name)
        assertEquals("Device dev_1.abc", server.takeRequest().headers["Authorization"])
    }

    @Test fun reportsTheHubsOwnWordsOnRefusal() = runBlocking {
        server.enqueue(MockResponse.Builder().code(401).body("That sign-in code has expired.\n").build())
        try {
            Hub(server.url("/").toString().trimEnd('/')).signIn("old", "x")
            fail("a refusal should throw")
        } catch (e: HubException) {
            assertEquals(401, e.status)
            assertEquals("That sign-in code has expired.", e.message)
        }
    }
}
