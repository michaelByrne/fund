import os
import random
from datetime import datetime, timedelta

import psycopg2
from dateutil.relativedelta import relativedelta  # For adding months
from faker import Faker
from psycopg2.extras import execute_values

# Database connection settings
DB_CONFIG = {
    'dbname': os.getenv("PG_DB"),
    'user': os.getenv("PG_USER"),
    'password': os.getenv("PG_PASS"),
    'host': os.getenv("PG_HOST"),
    'port': os.getenv("PG_PORT")
}

# Initialize Faker
fake = Faker()

fund_names = ["roxy's dental fund", "bco general fund", "server costs", "whelky homecoming fund",
              "bco fund server costs", "gaza relief fund"]


# Helper function to generate random dates within the past 12 months
def random_date_within_last_12_months():
    start_date = datetime.now() - timedelta(days=365)
    end_date = datetime.now()
    return fake.date_time_between(start_date=start_date, end_date=end_date)


# Helper function to add one month to a given date
def add_months(date, months):
    return date + relativedelta(months=months)


# Seed database
try:
    # Connect to the database
    connection = psycopg2.connect(**DB_CONFIG)
    cursor = connection.cursor()

    # Insert members with a created timestamp within the past 12 months
    members = [
        (fake.uuid4(), fake.name(), fake.email(), fake.name(), fake.name(), random_date_within_last_12_months())
        for _ in range(50)
    ]
    execute_values(cursor, "INSERT INTO member (id, bco_name, email, first_name, last_name, created) VALUES %s",
                   members)

    # Insert funds with a created timestamp within the past 12 months
    funds = []
    for fund_name in fund_names:
        created_timestamp = random_date_within_last_12_months()
        next_payment_timestamp = add_months(created_timestamp, 1)
        payout_frequency = random.choice(["once", "monthly"])

        funds.append(
            (
                fake.uuid4(),
                fund_name,
                fake.bs(),
                fake.bs(),
                "paypal",
                payout_frequency,
                created_timestamp,
                next_payment_timestamp
            )
        )

    execute_values(cursor,
                   "INSERT INTO fund (id, name, description, provider_id, provider_name, payout_frequency, created, next_payment) VALUES %s",
                   funds)

    # Get fund IDs and their payout frequencies
    cursor.execute("SELECT id, payout_frequency FROM fund")
    fund_data = cursor.fetchall()
    fund_ids = [row[0] for row in fund_data]
    monthly_fund_ids = [row[0] for row in fund_data if row[1] == "monthly"]

    # Get member IDs
    cursor.execute("SELECT id FROM member")
    member_ids = [row[0] for row in cursor.fetchall()]

    # Insert donation plans
    INTERVAL_UNITS = ['WEEK', 'MONTH']
    unique_plans = {}
    NUM_PLANS = 10

    while len(unique_plans) < NUM_PLANS:
        amount_cents = random.randint(1000, 10000)  # Random amount between $10 and $100
        interval_unit = random.choice(INTERVAL_UNITS)
        fund_id = random.choice(monthly_fund_ids)

        if (amount_cents, interval_unit, fund_id) not in unique_plans:
            unique_plans[(amount_cents, interval_unit, fund_id)] = (
                fake.uuid4(),
                f"Plan {len(unique_plans) + 1}",
                fake.uuid4(),
                amount_cents,
                interval_unit,
                random.randint(1, 12),
                random.choice([True, False]),
                datetime.now(),
                datetime.now(),
                fund_id
            )

    donation_plans = list(unique_plans.values())
    execute_values(cursor,
                   "INSERT INTO donation_plan (id, name, paypal_plan_id, amount_cents, interval_unit, interval_count, active, created, updated, fund_id) VALUES %s",
                   donation_plans)

    # Get donation plan IDs and their associated fund IDs
    cursor.execute("SELECT id, fund_id FROM donation_plan")
    plan_fund_map = {row[0]: row[1] for row in cursor.fetchall()}

    # Insert donations with a created timestamp within the past 12 months
    donations = []
    for _ in range(500):
        has_plan = random.choice([True, False])  # About half of donations will have a plan
        if has_plan:
            plan_id = random.choice(list(plan_fund_map.keys()))
            fund_id = plan_fund_map[plan_id]
        else:
            plan_id = None
            fund_id = random.choice(fund_ids)

        donation_id = fake.uuid4()
        donations.append(
            (
                donation_id,
                random.choice(member_ids),
                fund_id,
                "666",
                True,
                random_date_within_last_12_months(),
                plan_id
            )
        )

    execute_values(cursor,
                   "INSERT INTO donation (id, donor_id, fund_id, provider_order_id, active, created, donation_plan_id) VALUES %s",
                   donations)

    # Ensure every donation has at least one payment
    donation_payments = []

    for donation in donations:
        donation_id, plan_id, created_date = donation[0], donation[6], donation[5]

        if plan_id:
            cursor.execute("SELECT interval_unit, amount_cents FROM donation_plan WHERE id = %s", (plan_id,))
            plan_data = cursor.fetchone()

            if plan_data:
                interval_unit, amount_cents = plan_data

                next_payment_date = created_date
                generated_dates = set()  # Keep track of already generated payment timestamps

                # Generate payments until we reach or exceed the current date
                while True:
                    if next_payment_date >= datetime.now():
                        break  # Stop if the next interval would be in the future

                    if next_payment_date not in generated_dates:
                        donation_payments.append(
                            (
                                fake.uuid4(),
                                donation_id,
                                amount_cents,
                                "paypal",
                                next_payment_date
                            )
                        )
                        generated_dates.add(next_payment_date)

                    # Increment interval according to plan type
                    if interval_unit == "WEEK":
                        next_payment_date += timedelta(weeks=1)
                    elif interval_unit == "MONTH":
                        next_payment_date = add_months(next_payment_date, 1)

        else:
            # For one-time donations (no plan), create a single payment
            donation_payments.append(
                (
                    fake.uuid4(),
                    donation_id,
                    random.randint(1000, 10000),
                    "paypal",
                    created_date
                )
            )

    execute_values(cursor,
                   "INSERT INTO donation_payment (id, donation_id, amount_cents, paypal_payment_id, created) VALUES %s",
                   donation_payments)

    # Commit the transaction
    connection.commit()
    print("Database seeded successfully!")

except Exception as e:
    print(f"An error occurred: {e}")
    if connection:
        connection.rollback()

finally:
    if cursor:
        cursor.close()
    if connection:
        connection.close()
