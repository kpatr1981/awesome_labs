"""Order pipeline: workflow Order on task queue orders (reference solution of prog-147)."""
import asyncio
from concurrent.futures import ThreadPoolExecutor
from datetime import timedelta

from temporalio import activity, workflow
from temporalio.client import Client
from temporalio.common import RetryPolicy
from temporalio.exceptions import ActivityError, ApplicationError
from temporalio.worker import Worker

with workflow.unsafe.imports_passed_through():
    import shop


def key(what: str) -> str:
    """An idempotency key that is the same for every attempt of this workflow's WHAT."""
    return f"{activity.info().workflow_id}-{what}"


@activity.defn
def reserve(order: str) -> str:
    return shop.post("/step/reserve", {"run": order}, timeout=4)["done"]


@activity.defn
def release(order: str) -> str:
    return shop.post("/step/release", {"run": order}, timeout=4)["done"]


@activity.defn
def charge(order: str, amount: int) -> str:
    try:
        return shop.post("/charge", {"order": order, "amount": amount}, key=key("charge"), timeout=4)["charge"]
    except shop.ServiceError as e:
        if e.status == 402:
            raise ApplicationError(f"card declined for {order}", type="CardDeclined", non_retryable=True)
        raise


@activity.defn
def refund(order: str) -> str:
    return shop.post("/refund", {"order": order}, key=key("refund"), timeout=4)["refunded"]


@activity.defn
def book_courier(order: str) -> str:
    try:
        return shop.post("/book/courier", {"order": order}, timeout=4)["booking"]
    except shop.ServiceError as e:
        if e.status == 409:
            raise ApplicationError(f"no courier for {order}", type="NoCourier", non_retryable=True)
        raise


OPTS = dict(start_to_close_timeout=timedelta(seconds=5),
            retry_policy=RetryPolicy(initial_interval=timedelta(seconds=1), backoff_coefficient=2,
                                     maximum_interval=timedelta(seconds=10)))


@workflow.defn
class Order:
    def __init__(self) -> None:
        self.state, self.approver = "new", None

    @workflow.signal
    def approve(self, name: str) -> None:
        self.approver = name

    @workflow.query
    def status(self) -> str:
        return self.state

    @workflow.run
    async def run(self, o: dict) -> str:
        order, amount = o["order"], o["amount"]
        await workflow.execute_activity(reserve, order, **OPTS)
        self.state = "reserved"
        if amount > 500:
            self.state = "awaiting approval"
            try:
                await workflow.wait_condition(lambda: self.approver is not None,
                                              timeout=timedelta(seconds=o.get("wait_seconds", 86400)))
            except asyncio.TimeoutError:
                await workflow.execute_activity(release, order, **OPTS)
                self.state = "expired"
                return "expired"
        try:
            await workflow.execute_activity(charge, args=[order, amount], **OPTS)
        except ActivityError:
            await workflow.execute_activity(release, order, **OPTS)
            self.state = "failed"
            raise
        self.state = "charged"
        try:
            booking = await workflow.execute_activity(book_courier, order, **OPTS)
        except ActivityError:
            await workflow.execute_activity(refund, order, **OPTS)
            await workflow.execute_activity(release, order, **OPTS)
            self.state = "failed"
            raise
        self.state = "shipped"
        return f"shipped: {booking}"


async def main():
    client = await Client.connect("127.0.0.1:7233")
    worker = Worker(client, task_queue="orders", workflows=[Order],
                    activities=[reserve, release, charge, refund, book_courier],
                    activity_executor=ThreadPoolExecutor(10))
    print("worker polling task queue orders", flush=True)
    await worker.run()


if __name__ == "__main__":
    asyncio.run(main())
