# wls-ingress
An ingress controller that handles application generated cookies

This ingress controller uses a redis HA (sentinel) service to distribute the sticky sessions across multiple ingress controllers.
